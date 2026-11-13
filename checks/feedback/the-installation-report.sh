#!/usr/bin/env bash
# The checks described by the-installation-report.std.md, one function per case.
# Each function prints why it failed and returns non-zero; checks/run.sh reports them.

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
installed_form="$root/.github/ISSUE_TEMPLATE/i-installed-yoke.yml"

# std: yoke:the-installation-report.01
check_the_form_asks_three_things_and_nothing_else() {
  [[ -f "$installed_form" ]] || { echo "no .github/ISSUE_TEMPLATE/i-installed-yoke.yml"; return 1; }
  # Read without a YAML library: the form is written flat, one key per line, each element opened by `- type:`.
  python3 - "$installed_form" <<'PY'
import re, sys
text = open(sys.argv[1]).read()
wrong = []
def top(key):
    m = re.search(r'^%s:\s*"?([^"\n]*)"?\s*$' % key, text, re.M)
    return m.group(1).strip() if m else None
if top("name") != "I installed Yoke": wrong.append("its name is %r" % top("name"))
if top("title") != "I installed Yoke": wrong.append("its issues are titled %r" % top("title"))
if not re.search(r'^labels:\s*\[\s*"?installed"?\s*\]\s*$', text, re.M): wrong.append("its issues are not labelled installed")
body = text[text.index("\nbody:"):] if "\nbody:" in text else ""
elements = re.split(r'^  - type:\s*', body, flags=re.M)[1:]
asked = []
for e in elements:
    kind = e.split("\n", 1)[0].strip()
    if kind == "markdown":
        continue
    ident = re.search(r'^    id:\s*(\S+)', e, re.M)
    required = re.search(r'^      required:\s*true\s*$', e, re.M)
    asked.append((kind, ident.group(1) if ident else None, bool(required), e))
if [(k, i) for k, i, _, _ in asked] != [("dropdown", "form"), ("input", "where"), ("input", "release")]:
    wrong.append("it asks %s, not form, where and release" % [(k, i) for k, i, _, _ in asked])
for k, i, r, _ in asked:
    if not r: wrong.append("%s is not required" % i)
form = next((e for k, i, _, e in asked if i == "form"), "")
options = re.findall(r'^        - (.+)$', form, re.M)
if len(options) != 2 or not any("application form" in o.lower() for o in options) or not any("service form" in o.lower() for o in options):
    wrong.append("form offers %s, not the application form and the service form" % options)
if wrong:
    print("; ".join(wrong)); sys.exit(1)
PY
}

# std: yoke:the-installation-report.02
check_the_form_is_offered_beside_a_blank_issue() {
  local config="$root/.github/ISSUE_TEMPLATE/config.yml"
  [[ ! -f "$config" ]] && return 0
  grep -qE '^blank_issues_enabled:\s*false' "$config" && { echo "config.yml turns blank issues off"; return 1; }
  return 0
}
