# Blink Labs clanker workspace
#
# Workspace-level targets only. Source changes belong in the submodule under
# repos/ and use that project's own Makefile.

.PHONY: help validate scan-prs submodules status

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

validate: ## Validate the agent toolkit (manifests, skills, commands, hooks)
	@scripts/validate-toolkit.sh

scan-prs: ## Scan open PRs for current-head reviews, bots, approvals, and checks
	@python3 scripts/scan-prs.py $(ARGS)

submodules: ## Initialize or update every submodule checkout
	@git submodule update --init --recursive

status: ## Show workspace and submodule state
	@git status --short
	@echo
	@git submodule status --recursive
