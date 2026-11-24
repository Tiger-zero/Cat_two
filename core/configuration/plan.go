package configuration

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/core/instancesetting"
	coreuser "github.com/usememos/memos/core/user"
	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

// Reader is what a plan reads from the instance. *store.Store implements it.
// Every read bypasses caches, so a plan sees what is stored.
type Reader interface {
	GetConfigurationState(ctx context.Context) (*storepb.InstanceConfigurationState, error)
	GetUncachedInstanceSetting(ctx context.Context, find *store.FindInstanceSetting) (*storepb.InstanceSetting, error)
	ListUsers(ctx context.Context, find *store.FindUser) ([]*store.User, error)
}

// Plan is what apply would change.
type Plan struct {
	// Message is the plan as printed and as --json returns it.
	Message *apipb.Plan
	// next is the state record apply writes; nil when the plan has errors.
	next *storepb.InstanceConfigurationState
	// operations are the writes apply makes, in apply order.
	operations []*operation
}

// operation is what apply writes for one resource change.
type operation struct {
	change *apipb.ResourceChange
	kind   Kind

	// The instance setting to write, or the key to reset.
	setting    *storepb.InstanceSetting
	settingKey storepb.InstanceSettingKey

	// The managed fields of the declared administrator and the actual user, if any.
	user       *apipb.User
	actualUser *store.User
}

// Valid reports whether the plan can be applied.
func (p *Plan) Valid() bool {
	return len(p.Message.Errors) == 0
}

// HasChanges reports whether apply would write anything.
func (p *Plan) HasChanges() bool {
	return p.Message.StateRecordChanges
}

// Apply supported instance settings before updating the administrator.
const (
	rankInstanceSetting = iota
	rankUser
)

type rankedChange struct {
	rank   int
	change *apipb.ResourceChange
	op     *operation
}

// ComputePlan compares the desired state with what reader holds and returns
// what apply would do. It never writes. Problems that stop an apply, such as
// a managed resource no file declares any longer, are in the plan's errors.
func ComputePlan(ctx context.Context, desired *Desired, reader Reader) (*Plan, error) {
	state, err := reader.GetConfigurationState(ctx)
	if err != nil {
		return nil, err
	}
	p := &planner{
		ctx:       ctx,
		desired:   desired,
		reader:    reader,
		state:     state,
		managed:   map[string]*storepb.InstanceConfigurationState_ManagedResource{},
		lifecycle: map[string]*apipb.Lifecycle{},
		removed:   map[string]*apipb.Removed{},
		message:   &apipb.Plan{Serial: state.Serial, Warnings: slices.Clone(desired.Warnings)},
	}
	for _, resource := range state.Resources {
		p.managed[resource.Name] = resource
	}
	for _, lifecycle := range desired.Configuration.Lifecycle {
		p.lifecycle[lifecycle.Resource] = lifecycle
	}
	for _, removed := range desired.Configuration.Removed {
		p.removed[removed.Resource] = removed
	}

	steps := []func() error{p.planInstanceSettings, p.planUsers, p.planUndeclared}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return p.finish(), nil
}

type planner struct {
	ctx       context.Context
	desired   *Desired
	reader    Reader
	state     *storepb.InstanceConfigurationState
	managed   map[string]*storepb.InstanceConfigurationState_ManagedResource
	lifecycle map[string]*apipb.Lifecycle
	removed   map[string]*apipb.Removed
	message   *apipb.Plan
	changes   []rankedChange
	next      []*storepb.InstanceConfigurationState_ManagedResource

	plannedAdmin string
}

func (p *planner) fail(resource string, format string, args ...any) {
	p.message.Errors = append(p.message.Errors, resource+": "+fmt.Sprintf(format, args...))
}

// declared records the action for a declared resource from its field changes
// and whether it exists, and the state record entry apply would write.
func (p *planner) declared(rank int, name string, kind Kind, exists bool, changes []*apipb.FieldChange, entry *storepb.InstanceConfigurationState_ManagedResource, op *operation) {
	managed := p.managed[name]
	change := &apipb.ResourceChange{Resource: name, Source: p.desired.Sources[name], FieldChanges: changes}
	switch {
	case !exists:
		change.Action = apipb.ResourceChange_CREATE
		if managed != nil {
			change.Drift = true
			change.Message = "deleted outside the configuration since the last apply"
		}
	case managed == nil:
		change.Action = apipb.ResourceChange_ADOPT
	case len(changes) > 0:
		change.Action = apipb.ResourceChange_UPDATE
		// The declaration is what was last applied, so the instance changed.
		change.Drift = managed.Digest == entry.Digest
	default:
		change.Action = apipb.ResourceChange_NO_OP
	}

	lifecycle := p.lifecycle[name]
	entry.Name = name
	entry.Kind = string(kind)
	entry.Source = p.desired.Sources[name]
	entry.IgnoredFields = slices.Clone(lifecycle.GetIgnoreChanges())
	entry.PreventDestroy = lifecycle.GetPreventDestroy()
	if change.Action == apipb.ResourceChange_NO_OP && managed != nil {
		entry.AppliedTs = managed.AppliedTs
	}
	op.change, op.kind = change, kind
	p.changes = append(p.changes, rankedChange{rank: rank, change: change, op: op})
	p.next = append(p.next, entry)
}

func (p *planner) planInstanceSettings() error {
	for _, declared := range sortedByName(p.desired.Configuration.InstanceSettings, (*apipb.InstanceSetting).GetName) {
		if retiredResource(declared.Name) {
			continue
		}
		desiredStore := instancesetting.ToStore(declared)
		instancesetting.Normalize(desiredStore)
		if legacyStorage(desiredStore.GetStorageSetting()) {
			p.message.Warnings = append(p.message.Warnings, declared.Name+" is ignored because Cat_two only configures local attachment storage")
			continue
		}
		actual, err := p.reader.GetUncachedInstanceSetting(p.ctx, &store.FindInstanceSetting{Name: desiredStore.Key.String()})
		if err != nil {
			return errors.Wrapf(err, "failed to read %s", declared.Name)
		}
		desiredAPI := instancesetting.FromStore(desiredStore)
		var actualAPI proto.Message
		ignored := slices.Clone(p.lifecycle[declared.Name].GetIgnoreChanges())
		if desiredStore.Key == storepb.InstanceSettingKey_GENERAL {
			ignored = append(ignored, "generalSetting.disallowUserRegistration", "generalSetting.disallowPasswordAuth")
		}
		op := &operation{setting: desiredStore, settingKey: desiredStore.Key}
		if actual != nil {
			if legacyStorage(actual.GetStorageSetting()) {
				if managed := p.managed[declared.Name]; managed != nil {
					p.next = append(p.next, managed)
				}
				p.message.Warnings = append(p.message.Warnings, declared.Name+" is unchanged to preserve existing non-local storage metadata")
				continue
			}
			instancesetting.Normalize(actual)
			actualAPI = instancesetting.FromStore(actual)
			if len(ignored) > 0 {
				merged := proto.CloneOf(desiredAPI)
				copyFields(merged, actualAPI, ignored)
				op.setting = instancesetting.ToStore(merged)
				instancesetting.Normalize(op.setting)
			}
		}
		changes := diffResource(actualAPI, desiredAPI, ignored)
		entry := &storepb.InstanceConfigurationState_ManagedResource{
			ManagedFields: []string{"*"},
			Digest:        digest(desiredAPI, p.lifecycle[declared.Name]),
		}
		p.declared(rankInstanceSetting, declared.Name, KindInstanceSetting, actual != nil, changes, entry, op)
	}
	return nil
}

// legacyStorage recognizes persisted non-local storage without reading its credentials.
func legacyStorage(setting *storepb.InstanceStorageSetting) bool {
	return setting.GetS3Config() != nil || setting.GetStorageType() == storepb.InstanceStorageSetting_S3 ||
		setting.GetStorageType() == storepb.InstanceStorageSetting_DATABASE || slices.ContainsFunc(setting.GetStorages(), func(storage *storepb.Storage) bool {
		return storage.GetType() != storepb.StorageType_STORAGE_TYPE_LOCAL
	})
}

func (p *planner) planUsers() error {
	declaredUsers := p.desired.Configuration.Users
	if len(declaredUsers) == 0 {
		return nil
	}
	users, err := p.reader.ListUsers(p.ctx, &store.FindUser{})
	if err != nil {
		return errors.Wrap(err, "failed to read users")
	}
	adminByName := map[string]*store.User{}
	for _, user := range users {
		if user.Role == store.RoleAdmin && user.RowStatus != store.Archived {
			adminByName[user.Username] = user
		}
	}
	eligible := make([]*apipb.User, 0, len(declaredUsers))
	for _, user := range declaredUsers {
		if user.Role == apipb.User_ADMIN && (len(users) == 0 || adminByName[user.Username] != nil) {
			eligible = append(eligible, user)
		}
	}
	if len(eligible) > 1 && len(users) > 0 {
		managed := slices.DeleteFunc(slices.Clone(eligible), func(user *apipb.User) bool { return p.managed[user.Name] == nil })
		if len(managed) == 1 {
			eligible = managed
		}
	}
	if len(eligible) != 1 {
		p.message.Warnings = append(p.message.Warnings, "user declarations are ignored: Cat_two configures only one administrator and never creates an additional account")
		return nil
	}
	declared := eligible[0]
	if len(declaredUsers) > 1 {
		p.message.Warnings = append(p.message.Warnings, "additional user declarations are ignored; only the existing administrator can be configured")
	}
	name := declared.Name
	actual := adminByName[declared.Username]
	lifecycle := p.lifecycle[name]
	optional := p.desired.UserFields[name]
	version := lifecycle.GetWriteOnlyVersion()
	recorded := p.managed[name].GetWriteOnlyVersion()
	sendPassword := actual == nil || version > recorded
	if sendPassword && declared.Password == "" {
		if actual == nil {
			p.fail(name, "password is required to create the user")
		} else {
			p.fail(name, "writeOnlyVersion is %d, above the %d last applied, but no password is declared", version, recorded)
		}
		return nil
	}
	managedView := managedUserView(declared, optional)
	changes := p.userChanges(managedView, actual, sendPassword, version)
	entry := &storepb.InstanceConfigurationState_ManagedResource{
		ManagedFields:    append([]string{"username", "role"}, optional...),
		Digest:           digest(managedView, lifecycle),
		WriteOnlyVersion: max(version, recorded),
	}
	p.plannedAdmin = name
	p.declared(rankUser, name, KindUser, actual != nil, changes, entry, &operation{user: managedView, actualUser: actual})
	return nil
}

// managedUserView is the part of a declared user the configuration manages.
func managedUserView(declared *apipb.User, optional []string) *apipb.User {
	view := &apipb.User{Name: declared.Name, Username: declared.Username, Role: declared.Role, Password: declared.Password}
	if slices.Contains(optional, "displayName") {
		view.DisplayName = declared.DisplayName
	}
	if slices.Contains(optional, "email") {
		// Layer 2 already rejected an email that does not normalize.
		view.Email, _ = store.NormalizeEmail(declared.Email)
	}
	return view
}

func (p *planner) userChanges(view *apipb.User, actual *store.User, sendPassword bool, version int32) []*apipb.FieldChange {
	ignored := p.lifecycle[view.Name].GetIgnoreChanges()
	optional := p.desired.UserFields[view.Name]
	changes := []*apipb.FieldChange{}
	add := func(path, before, after string) {
		if slices.Contains(ignored, path) {
			return
		}
		if before != after {
			changes = append(changes, &apipb.FieldChange{Path: path, Before: before, After: after})
		}
	}
	if actual == nil {
		add("username", "", strconv.Quote(view.Username))
		add("role", "", view.Role.String())
		for _, field := range optional {
			if field == "displayName" {
				add(field, "", strconv.Quote(view.DisplayName))
			} else {
				add(field, "", strconv.Quote(view.Email))
			}
		}
	} else {
		add("role", coreuser.RoleFromStore(actual.Role).String(), view.Role.String())
		for _, field := range optional {
			if field == "displayName" {
				add(field, strconv.Quote(actual.Nickname), strconv.Quote(view.DisplayName))
			} else {
				add(field, strconv.Quote(actual.Email), strconv.Quote(view.Email))
			}
		}
	}
	if sendPassword {
		before := ""
		if actual != nil {
			before = writeOnlyPlaceholder
		}
		changes = append(changes, &apipb.FieldChange{Path: "password", Before: before, After: fmt.Sprintf("(write-only, version %d)", version)})
	}
	return changes
}

// planUndeclared handles managed resources no file declares any longer: a
// removed entry destroys, resets, or releases them; without one, removal was
// not explicit and the plan fails.
func (p *planner) planUndeclared() error {
	for _, managed := range p.state.Resources {
		name := managed.Name
		if retiredResource(name) || p.desired.preserved[name] {
			p.next = append(p.next, managed)
			continue
		}
		if name == p.plannedAdmin {
			continue
		}
		kind, id, err := ParseResourceName(name)
		if kind == KindUser {
			removed := p.removed[name]
			if removed == nil || removed.Destroy {
				p.next = append(p.next, managed)
				continue
			}
			change := &apipb.ResourceChange{Resource: name, Action: apipb.ResourceChange_RELEASE}
			p.changes = append(p.changes, rankedChange{rank: rankUser, change: change, op: &operation{change: change}})
			continue
		}
		if _, declared := p.desired.Sources[name]; declared {
			continue
		}

		if err != nil {
			p.fail(name, "the state record names a resource this version cannot manage: %v", err)
			continue
		}
		removed := p.removed[name]
		if id == storepb.InstanceSettingKey_STORAGE.String() && (removed == nil || removed.Destroy) {
			actual, err := p.reader.GetUncachedInstanceSetting(p.ctx, &store.FindInstanceSetting{Name: id})
			if err != nil {
				return errors.Wrapf(err, "failed to read %s", name)
			}
			if legacyStorage(actual.GetStorageSetting()) {
				p.next = append(p.next, managed)
				p.message.Warnings = append(p.message.Warnings, name+" is unchanged to preserve existing non-local storage metadata")
				continue
			}
		}
		change := &apipb.ResourceChange{Resource: name}
		op := &operation{change: change, kind: kind}
		switch {
		case removed == nil:
			change.Action = apipb.ResourceChange_ERROR
			change.Message = "managed but no longer declared; add a removed entry to reset or release it"
			p.fail(name, "%s", change.Message)
		case !removed.Destroy:
			change.Action = apipb.ResourceChange_RELEASE
		case managed.PreventDestroy:
			change.Action = apipb.ResourceChange_ERROR
			change.Message = "preventDestroy was set when last applied; apply once without it, then destroy"
			p.fail(name, "%s", change.Message)
		default:
			change.Action = apipb.ResourceChange_RESET
			op.settingKey = storepb.InstanceSettingKey(storepb.InstanceSettingKey_value[id])
		}
		p.changes = append(p.changes, rankedChange{rank: rankInstanceSetting, change: change, op: op})
	}
	return nil
}

func (p *planner) finish() *Plan {
	slices.SortStableFunc(p.changes, func(a, b rankedChange) int {
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		return strings.Compare(a.change.Resource, b.change.Resource)
	})
	writes := false
	plan := &Plan{Message: p.message}
	for _, ranked := range p.changes {
		p.message.ResourceChanges = append(p.message.ResourceChanges, ranked.change)
		plan.operations = append(plan.operations, ranked.op)
		if ranked.change.Action != apipb.ResourceChange_NO_OP {
			writes = true
		}
	}
	if !plan.Valid() {
		return plan
	}

	slices.SortFunc(p.next, func(a, b *storepb.InstanceConfigurationState_ManagedResource) int {
		return strings.Compare(a.Name, b.Name)
	})
	plan.next = &storepb.InstanceConfigurationState{
		Serial:       p.state.Serial,
		ConfigDigest: digest(p.desired.Configuration),
		Resources:    p.next,
	}
	p.message.StateRecordChanges = writes || !sameRecord(p.state, plan.next)
	return plan
}

// sameRecord compares the managed resources of two state records, ignoring
// the apply metadata a plan does not compute. The config digest is left out:
// it is informational, so a change to it alone, such as a deleted leftover
// removed entry, is not worth an apply.
func sameRecord(a, b *storepb.InstanceConfigurationState) bool {
	strip := func(state *storepb.InstanceConfigurationState) *storepb.InstanceConfigurationState {
		stripped := &storepb.InstanceConfigurationState{}
		for _, resource := range state.Resources {
			resource = proto.CloneOf(resource)
			resource.AppliedTs = 0
			stripped.Resources = append(stripped.Resources, resource)
		}
		slices.SortFunc(stripped.Resources, func(x, y *storepb.InstanceConfigurationState_ManagedResource) int {
			return strings.Compare(x.Name, y.Name)
		})
		return stripped
	}
	return proto.Equal(strip(a), strip(b))
}

func sortedByName[T any](items []T, name func(T) string) []T {
	sorted := slices.Clone(items)
	slices.SortFunc(sorted, func(a, b T) int { return strings.Compare(name(a), name(b)) })
	return sorted
}
