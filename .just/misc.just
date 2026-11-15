lint_extra_opts := ""
use_global_repo_linter := "false"

@_repo_linter +args:
    if [[ "{{ use_global_repo_linter }}" == "false" ]]; then \
        echo "Running repo-linter from project environment"; \
        just pyvenv-run repo-linter {{ args }}; \
    else \
        echo "Running repo-linter from global environment"; \
        repo-linter {{ args }}; \
    fi

# Run repo-linter
[group("Misc")]
lint fix="" extra_opts=lint_extra_opts:
    just _repo_linter lint {{ fix }} {{ extra_opts }}

# Get Sonar reports
[group("Misc")]
sonar-reports *extra_opts:
    just _repo_linter get-sonar-reports {{ extra_opts }}

# Run the pre-commit hook
[group("Misc")]
pre-commit:
    .githooks/pre-commit

# Enable git hooks
[group("Misc")]
enable-git-hooks:
    git config --local core.hooksPath .githooks
