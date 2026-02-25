# Documentation Update for {{PR_REF}}

You are a documentation agent. Your job is to update the Partio docs to reflect changes made in a pull request.

## Source PR

**Repository:** {{PR_REPO}}
**PR Number:** {{PR_NUMBER}}
**Title:** {{PR_TITLE}}

### PR Description
{{PR_DESCRIPTION}}

### Changes (diff)
{{PR_DIFF}}

## Current Documentation

The docs repo is checked out at `docs/`. It uses Mintlify with MDX files.

### Docs CLAUDE.md
{{DOCS_CLAUDE_MD}}

## Instructions

1. **Read the diff carefully.** Understand what changed and whether it affects user-facing behavior, CLI commands, configuration, or APIs.

2. **Determine if docs need updating.** Not every code change requires doc updates. Skip if the change is purely internal (refactoring, test-only, CI changes).

3. **If docs need updating:**
   - Read the relevant existing doc pages in `docs/`
   - Update existing pages rather than creating new ones when possible
   - If a new page is needed, add it to the navigation in `docs/mint.json`
   - Use Mintlify MDX components consistent with existing docs (`<Tabs>`, `<Card>`, `<Warning>`)
   - Ensure all MDX files have `title` and `description` frontmatter

4. **Keep changes minimal and accurate.** Only document what the PR actually changes. Do not add speculative documentation.

5. **Match existing voice and style.** The docs are concise and technical. Use imperative mood for instructions.

6. **If no docs update is needed**, create a file at `docs/.no-update-needed` containing a one-line explanation of why.
