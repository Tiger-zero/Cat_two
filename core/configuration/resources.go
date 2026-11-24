package configuration

import (
	"fmt"

	"github.com/usememos/memos/core/instancesetting"
	"github.com/usememos/memos/store"
)

// checkResources runs the rules each resource must meet before it is stored,
// the same rules the API applies, on every merged resource. A declared value
// is the whole value, so nothing is exempt as unchanged.
func checkResources(desired *Desired) []string {
	var problems []string
	report := func(name string, err error) {
		problems = append(problems, fmt.Sprintf("%s: %s: %v", desired.Sources[name], name, err))
	}
	config := desired.Configuration
	for _, setting := range config.InstanceSettings {
		stored := instancesetting.ToStore(setting)
		if err := instancesetting.Validate(stored); err != nil {
			report(setting.GetName(), err)
		}
	}
	for _, user := range config.Users {
		if user.GetEmail() != "" {
			if _, err := store.NormalizeEmail(user.GetEmail()); err != nil {
				report(user.GetName(), err)
			}
		}
	}
	return problems
}
