---
description: Update changelog for unreleased changes
---
Latest tag: !`git describe --tags --abbrev=0 --match 'v*'`
Recent commits since latest tag: !`git log --oneline $(git describe --tags --abbrev=0 --match 'v*')..HEAD`

Update `CHANGELOG.md` for Unreleased changes using only committed changes since the latest `v*` tag (ignore unstaged files). Add new bullets under the existing Unreleased sections; do not create a new version section, do not move or clear Unreleased, and do not update compare links.
Describe internal pipeline changes only in terms of external behavior; keep Deprecated entries but remove any "(since ...)" wording.

@CHANGELOG.md
