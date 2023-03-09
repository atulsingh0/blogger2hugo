#!/usr/bin/env bash
set -eu -o pipefail

branch=$(git symbolic-ref --short HEAD 2>/dev/null || echo unknown)
gitref=$(git rev-parse --short HEAD 2>/dev/null || echo latest)
do_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ "$branch" == "main" ]]; then
    _version="1.0.${CIRCLE_BUILD_NUM-0}-${gitref}"
elif [[ "$branch" == "canary" ]]; then
    _version="0.5.${CIRCLE_BUILD_NUM-0}-canary-${gitref}"
else
    _version="0.0.${CIRCLE_BUILD_NUM-0}-dev-${gitref}"
fi

reportDir="test-reports"
binaryDir="target"
testDir="test-output"

make-target() {
    mkdir -p "${binaryDir}"
    target="${binaryDir}/bin/$(go env GOOS)/$(go env GOARCH)"
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_binary="Build the binaries."
binary() {
    local ldflags
    ldflags="${1:-}" # Optionally accept build flags

    make-target

    local suffix=""
    if [ "$(go env GOOS)" = "windows" ]; then
        suffix=".exe"
    fi

    export CGO_ENABLED=0
    set -x
    go build -ldflags "$ldflags" -o "$target/${suffix}" main.go
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_build="Build the binaries for production"
build() {
    local date ldflags
    date=$(date "+%FT%T%z")
    ldflags="-s -w -X github.com/atulsingh0/blogger2hugo/cmd.Version=$_version -X github.com/atulsingh0/blogger2hugo/cmd.Date=$date"

    GOOS=linux GOARCH=amd64 binary "$ldflags" &
    GOOS=darwin GOARCH=amd64 binary "$ldflags" &

    local fail="0"
    for job in $(jobs -p); do
        wait "${job}" || ((fail += 1))
    done

    if [ "$fail" == "0" ]; then
        echo "Compilation succeeded"
    else
        echo "Compile failed ($fail)"
        exit 1
    fi

    mkdir -p "${binaryDir}"
    echo "$_version" | tee "${binaryDir}"/version.txt
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_lint="Run golanci-lint to lint go files."
lint() {
    exec ./bin/golangci-lint run "${@:-./...}"
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_lint_report="Run golanci-lint to lint go files and generate an xml report."
lint-report() {
    output="${reportDir}/lint.xml"
    echo "Storing results as Junit XML in ${output}" >&2
    mkdir -p "${reportDir}"

    lint ./... --out-format junit-xml | tee "${output}"
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_test="Run normal unit tests"
test() {
    mkdir -p "${reportDir}"
    # -count=1 is used to forcibly disable test result caching
    ./bin/gotestsum --junitfile="${reportDir}/junit.xml" -- -race -count=1 "${@:-./...}"
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_run="Run Binary"
run() {
    # -count=1 is used to forcibly disable test result caching
    go run main.go tests/data/story-blogger-backup.xml "${testDir}"
    ls -l "${testDir}"
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_godoc="Run godoc to read documentation."
godoc() {
    install-go-bin "golang.org/x/tools/cmd/godoc@v0.1.3"
    local url
    url=http://localhost:6060/pkg/github.com/atulsingh0/blogger2hugo/
    command -v xdg-open && xdg-open $url &
    command -v open && open $url &
    ./bin/godoc -http=127.0.0.1:6060
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_go_mod_tidy="Run 'go mod tidy' to clean up module files."
go-mod-tidy() {
    go mod tidy -v
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_install_devtools="Install tools required for doing dev stuff"
install-devtools() {
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.49.0

    install-go-bin \
        "gotest.tools/gotestsum@v1.7.0"
}

install-go-bin() {
    for pkg in "${@}"; do
        GOBIN="${PWD}/bin" go install "${pkg}" &
    done
    wait
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_create_stub_test_files="Create an empty pkg_test in all directories with no tests.

Creating this blank test file will ensure that coverage considers all
packages, not just those with tests.
"
create-stub-test-files() {
    # Variable expansion is not intended within the single quoted command
    # shellcheck disable=SC2016
    go list -f '{{if not .TestGoFiles}}{{.Name}} {{.Dir}}{{end}}' ./... |
        xargs -r --max-args=2 bash -c 'echo "package $0" > "$1/pkg_test.go"'
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_run_goimports="Run goimports for package"
run-goimports() {
    command -v ./bin/goimports || install-go-bin "golang.org/x/tools/cmd/goimports@v0.0.0-20201208183658-cc330816fc52"
    ./bin/goimports -local "github.com/circleci/feature-flags" -w "${@:-.}"
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_version="Print version"
version() {
    echo "$_version"
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_generate="generate any generated code"
generate() {
    go generate -x ./...
}

help-text-intro() {
    echo "
DO

A set of simple repetitive tasks that adds minimally
to standard tools used to build and test the service.
(e.g. go and docker)
"
}

### START FRAMEWORK ###
# Do Version 0.0.4
# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_self_update="Update the framework from a file.

Usage: $0 self-update FILENAME
"
self-update() {
    local source selfpath pattern
    source="$1"
    selfpath="${BASH_SOURCE[0]}"
    cp "$selfpath" "$selfpath.bak"
    pattern='/### START FRAMEWORK/,/END FRAMEWORK ###$/'
    (
        sed "${pattern}d" "$selfpath"
        sed -n "${pattern}p" "$source"
    ) \
        >"$selfpath.new"
    mv "$selfpath.new" "$selfpath"
    chmod --reference="$selfpath.bak" "$selfpath"
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_completion="Print shell completion function for this script.

Usage: $0 completion SHELL"
completion() {
    local shell
    shell="${1-}"

    if [ -z "$shell" ]; then
        echo "Usage: $0 completion SHELL" 1>&2
        exit 1
    fi

    case "$shell" in
    bash)
        (
            echo
            echo '_dotslashdo_completions() { '
            # shellcheck disable=SC2016
            echo '  COMPREPLY=($(compgen -W "$('"$0"' list)" "${COMP_WORDS[1]}"))'
            echo '}'
            echo 'complete -F _dotslashdo_completions '"$0"
        )
        ;;
    zsh)
        cat <<EOF
_dotslashdo_completions() {
  local -a subcmds
  subcmds=()
  DO_HELP_SKIP_INTRO=1 $0 help | while read line; do
EOF
        cat <<'EOF'
    cmd=$(cut -f1  <<< $line)
    cmd=$(awk '{$1=$1};1' <<< $cmd)

    desc=$(cut -f2- <<< $line)
    desc=$(awk '{$1=$1};1' <<< $desc)

    subcmds+=("$cmd:$desc")
  done
  _describe 'do' subcmds
}

compdef _dotslashdo_completions do
EOF
        ;;
    fish)
        cat <<EOF
complete -e -c do
complete -f -c do
for line in (string split \n (DO_HELP_SKIP_INTRO=1 $0 help))
EOF
        cat <<'EOF'
  set cmd (string split \t $line)
  complete -c do  -a $cmd[1] -d $cmd[2]
end
EOF
        ;;
    esac
}

list() {
    declare -F | awk '{print $3}'
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_clean="Clean the Binary and Test Output directory"
clean() {
    rm -rf "${binaryDir}" && echo "Deleted: ${binaryDir}"
    rm -rf "${testDir}" && echo "Deleted: ${testDir}"
}

# This variable is used, but shellcheck can't tell.
# shellcheck disable=SC2034
help_help="Print help text, or detailed help for a task."
help() {
    local item
    item="${1-}"
    if [ -n "${item}" ]; then
        local help_name
        help_name="help_${item//-/_}"
        echo "${!help_name-}"
        return
    fi

    if [ -z "${DO_HELP_SKIP_INTRO-}" ]; then
        type -t help-text-intro >/dev/null && help-text-intro
    fi
    for item in $(list); do
        local help_name text
        help_name="help_${item//-/_}"
        text="${!help_name-}"
        [ -n "$text" ] && printf "%-30s\t%s\n" "$item" "$(echo "$text" | head -1)"
    done
}

case "${1-}" in
list) list ;;
"" | "help") help "${2-}" ;;
*)
    if ! declare -F "${1}" >/dev/null; then
        printf "Unknown target: %s\n\n" "${1}"
        help
        exit 1
    else
        "$@"
    fi
    ;;
esac
### END FRAMEWORK ###
