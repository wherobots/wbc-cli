package commands

import (
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
	builder.WriteString(strings.Repeat("  ", depth))
	builder.WriteString(cmd.Name())
	builder.WriteString("\n")

	children := visibleChildren(cmd)
	slices.SortFunc(children, func(a, b *cobra.Command) int {
		return strings.Compare(a.Name(), b.Name())
	})

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
