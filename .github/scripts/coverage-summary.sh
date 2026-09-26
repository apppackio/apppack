#!/usr/bin/env bash
#
# Render a Go coverage profile as a per-package markdown table, least
# covered first. Writes to stdout; the workflow tees it into the job
# summary. Run it locally the same way:
#
#     make test && .github/scripts/coverage-summary.sh
#
# The total is summed from the profile rather than read off
# `go tool cover -func`. That tool only attributes blocks that land inside
# an *ast.FuncDecl, so every `Run: func(...)` hanging off a package-level
# `&cobra.Command{...}` is invisible to it -- cmd/ps.go contributes 98
# blocks to the profile and 4 functions to `-func`. Over the whole tree
# that is the difference between 22.2% and a reported 26.6%. Summing the
# profile agrees with the per-package figures `go test -cover` prints.
#
# Kept to POSIX awk: the GitHub runner has mawk, not the gawk most of us
# have locally, so no asort/asorti/length(array).
set -euo pipefail

profile="${1:-coverage.out}"
module="github.com/apppackio/apppack"

if [ ! -f "$profile" ]; then
	echo "No coverage profile was produced."
	exit 0
fi

echo "## Coverage"
echo

awk -v module="$module" '
	NR > 1 {
		# "<import path>/file.go:12.34,56.78 <statements> <count>"
		pkg = $1
		sub(/:[0-9]+\.[0-9]+,[0-9]+\.[0-9]+$/, "", pkg)
		sub(/\/[^\/]+$/, "", pkg)
		sub("^" module "/?", "", pkg)
		if (pkg == "") pkg = "(root)"

		stmts[pkg] += $2
		total += $2
		if ($3 > 0) {
			hit[pkg] += $2
			covered += $2
		}
	}

	END {
		if (total == 0) {
			print "The coverage profile is empty."
			exit
		}

		printf "**%.1f%%** of %d statements\n\n", covered * 100 / total, total
		print "| Package | Coverage | Statements |"
		print "|---|---:|---:|"

		n = 0
		for (p in stmts) {
			n = n + 1
			name[n] = p
		}

		# Insertion sort by coverage. Ties break on package name so the
		# table is byte-identical between runs -- `for (p in stmts)`
		# hashes differently in mawk and gawk, and six packages sit at
		# 0.0%.
		for (i = 2; i <= n; i++) {
			key = name[i]
			pct = hit[key] * 100 / stmts[key]
			j = i - 1
			while (j >= 1) {
				jpct = hit[name[j]] * 100 / stmts[name[j]]
				if (jpct < pct || (jpct == pct && name[j] < key)) break
				name[j + 1] = name[j]
				j = j - 1
			}
			name[j + 1] = key
		}

		for (i = 1; i <= n; i++) {
			p = name[i]
			printf "| `%s` | %.1f%% | %d |\n", p, hit[p] * 100 / stmts[p], stmts[p]
		}
	}
' "$profile"
