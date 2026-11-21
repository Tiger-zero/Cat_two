# Design Documents

This archive preserves design records inherited from Memos and their historical status labels.
The [Cat_two development guide](../../SKILL.md) describes current development practices.

A design document records a decision and the reasoning behind it. Source code, tests, and package `README.md` or `doc.go` files describe current
behavior; a design document does not have to.

## Lifecycle

- **Draft.** A design that is not yet shipped in a tagged release may change freely, even after its code is merged. Keep it in step with the
  implementation until the release, because the design itself often changes while a feature settles.
- **Implemented.** Once the feature ships in a release, set `Status: Implemented` and freeze the body. Do not edit it to track later code changes.
- **Amendments.** When a later release changes a frozen design, add or extend a dated `## Amendments` section at the end that states what changed and why.
  Leave the original body intact, so the original rationale stays readable. A change large enough to need its own rationale gets a new design document
  that links back to the one it replaces.

## Index

Implemented, awaiting release:

- [Markup: image annotation and saving](drawing-annotation-and-recovery.md)
- [MariaDB compatibility testing](mariadb-compatibility-testing.md)

Implemented and frozen:

- [API abuse controls](api-abuse-controls.md)
- [Declarative configuration](declarative-configuration.md)
- [Memo attachment and image filters](memo-attachment-filters.md)
- [Memos Export Format](memos-export-format.md), including the schemas under `memo-export/1.0/`
- [Multi-Spaces](multi-spaces.md)
- [Unique email](unique-email.md)
- [User usage limits](user-usage-limits.md)

## Historical links

- The Unique Email design cites `signup-abuse-controls.md` as “Signup Abuse Controls.” The file is absent from the recorded repository history. The archived API Abuse Controls design describes the [signup policy seam](api-abuse-controls.md#signup-policy-seam).
