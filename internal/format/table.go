// SPDX-License-Identifier: MIT

package format

import (
	"fmt"
	"strings"
	"text/tabwriter"
)

// RenderTable renders headers and rows as column-aligned text using tabwriter.
func RenderTable(m TableMarshaler) (string, error) {
	headers := m.TableHeaders()
	rows := m.TableRows()
	if len(headers) == 0 {
		return "", fmt.Errorf("table has no headers")
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))
	// The rule under each header is as wide as the header: a lone "-" reads as an empty
	// cell, the marker of a missing value.
	rule := make([]string, len(headers))
	for i, h := range headers {
		rule[i] = strings.Repeat("-", max(len(h), 1))
	}
	_, _ = fmt.Fprintln(tw, strings.Join(rule, "\t"))
	for _, row := range rows {
		if len(row) != len(headers) {
			return "", fmt.Errorf("table row has %d columns, want %d", len(row), len(headers))
		}
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return "", err
	}
	return b.String(), nil
}
