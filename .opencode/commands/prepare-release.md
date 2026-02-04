---
description: Prepare release changelog
---
Latest tag: !`git describe --tags --abbrev=0 --match 'v*'`
Recent commits since latest tag: !`git log --oneline $(git describe --tags --abbrev=0 --match 'v*')..HEAD`
Today: !`date +%Y-%m-%d`

Prepare `CHANGELOG.md` for a new release using only committed changes since the latest `v*` tag (ignore unstaged files). Create a new release section with today’s date and move the Unreleased entries into it. Keep the Deprecated section but remove any "(since ...)" wording from its entries. Describe internal pipeline changes only in terms of external behavior. Remove any empty headings within the new release section. Leave Unreleased empty. Update compare links to point `[unreleased]` at the new version and add a link for the new version against the previous tag.

@CHANGELOG.md
