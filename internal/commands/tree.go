package commands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func RenderTree(root *cobra.Command) string {
	var b strings.Builder
	renderNode(&b, root, 0)
	return strings.TrimRight(b.String(), "\n")
}

func renderNode(builder *strings.Builder, cmd *cobra.Command, depth int) {
	indent := strings.Repeat("  ", depth)
	children := visibleChildren(cmd)
	slices.SortFunc(children, func(a, b *cobra.Command) int {
		return strings.Compare(a.Name(), b.Name())
	})

	// For leaf nodes (verb commands) show the summary; for grouping nodes just the name.
	short := strings.TrimSpace(cmd.Short)
	if short != "" && len(children) == 0 {
		// Align: pad name to a minimum width relative to siblings when possible.
		fmt.Fprintf(builder, "%s%-20s %s\n", indent, cmd.Name(), short)
	} else {
		fmt.Fprintf(builder, "%s%s\n", indent, cmd.Name())
	}

	for _, child := range children {
		renderNode(builder, child, depth+1)
	}
}

func visibleChildren(cmd *cobra.Command) []*cobra.Command {
	children := cmd.Commands()
	filtered := make([]*cobra.Command, 0, len(children))
	for _, child := range children {
		if child == nil || child.Hidden {
			continue
		}
		name := child.Name()
		if name == "completion" || name == "help" {
			continue
		}
		filtered = append(filtered, child)
	}
	return filtered
}
