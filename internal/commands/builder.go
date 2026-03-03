package commands

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"wherobots/cli/internal/config"
	"wherobots/cli/internal/executor"
	"wherobots/cli/internal/hints"
	"wherobots/cli/internal/spec"
)

func BuildRootCommand(cfg config.Config, runtimeSpec *spec.RuntimeSpec) *cobra.Command {
	flags := &GlobalFlags{}
	client := &http.Client{Timeout: cfg.HTTPTimeout}
	operationByCommand := map[*cobra.Command]*spec.Operation{}
	resourceCommands := map[string]*cobra.Command{}

	var root *cobra.Command
	printTree := func(cmd *cobra.Command) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), RenderTree(root))
		return err
	}

	root = &cobra.Command{
		Use:           cfg.AppName,
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.Tree {
				return printTree(cmd)
			}
			return cmd.Help()
		},
	}

	root.PersistentFlags().StringVar(&flags.JSONBody, "json", "", "raw JSON request body")
	root.PersistentFlags().StringArrayVarP(&flags.Query, "query", "q", nil, "query pair (key=value), repeatable")
	root.PersistentFlags().BoolVar(&flags.DryRun, "dry-run", false, "print curl equivalent without executing request")
	root.PersistentFlags().BoolVar(&flags.Tree, "tree", false, "print available command tree")

	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return hints.Wrap(findOperationContext(operationByCommand, cmd), err)
	})

	for _, op := range runtimeSpec.Operations {
		parent := ensureResourceHierarchy(root, resourceCommands, PathToResourceSegments(op.Path))
		verb := uniqueVerbName(parent, ChooseVerb(op))
		op.Verb = verb
		op.CommandPath = append(PathToResourceSegments(op.Path), verb)

		methodCommand := buildOperationCommand(op, flags, cfg, runtimeSpec, client, printTree)
		methodCommand.Use = buildCommandUse(verb, op.PathParamOrder)
		operationByCommand[methodCommand] = op
		parent.AddCommand(methodCommand)
	}

	return root
}

func buildOperationCommand(
	op *spec.Operation,
	flags *GlobalFlags,
	cfg config.Config,
	runtimeSpec *spec.RuntimeSpec,
	client *http.Client,
	printTree func(cmd *cobra.Command) error,
) *cobra.Command {
	return &cobra.Command{
		Use:           op.Verb,
		Short:         fmt.Sprintf("%s %s", op.Method, op.Path),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			expected := len(op.PathParamOrder)
			if len(args) != expected {
				return hints.Wrap(op, fmt.Errorf("invalid argument count: expected %d path args %v, got %d", expected, op.PathParamOrder, len(args)))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.Tree {
				return printTree(cmd)
			}

			parsedQuery, err := ParseQueryPairs(flags.Query)
			if err != nil {
				return hints.Wrap(op, err)
			}

			queryPairs := make([]executor.QueryPair, 0, len(parsedQuery))
			for _, pair := range parsedQuery {
				queryPairs = append(queryPairs, executor.QueryPair{Key: pair.Key, Value: pair.Value})
			}

			req, err := executor.BuildRequest(cmd.Context(), cfg, runtimeSpec, op, args, queryPairs, flags.JSONBody)
			if err != nil {
				return hints.Wrap(op, err)
			}

			if flags.DryRun {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), executor.RenderCurl(req, flags.JSONBody))
				return err
			}

			body, err := executor.Do(client, req)
			if err != nil {
				return err
			}

			return executor.WriteSuccessResponse(cmd.OutOrStdout(), body)
		},
	}
}

func ensureResourceHierarchy(root *cobra.Command, cache map[string]*cobra.Command, segments []string) *cobra.Command {
	current := root
	keyParts := make([]string, 0, len(segments))
	for _, segment := range segments {
		keyParts = append(keyParts, segment)
		key := strings.Join(keyParts, "/")
		child, exists := cache[key]
		if !exists {
			segmentName := segment
			child = &cobra.Command{
				Use:           segmentName,
				SilenceUsage:  true,
				SilenceErrors: true,
				RunE: func(cmd *cobra.Command, args []string) error {
					return cmd.Help()
				},
			}
			cache[key] = child
			current.AddCommand(child)
		}
		current = child
	}
	return current
}

func uniqueVerbName(parent *cobra.Command, preferred string) string {
	if preferred == "" {
		preferred = "call"
	}
	name := preferred
	if parent.CommandPath() == name {
		name = preferred + "-1"
	}

	attempt := 1
	for hasChildNamed(parent, name) {
		attempt++
		name = fmt.Sprintf("%s-%d", preferred, attempt)
	}
	return name
}

func hasChildNamed(parent *cobra.Command, name string) bool {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return true
		}
	}
	return false
}

func buildCommandUse(verb string, pathParamOrder []string) string {
	if len(pathParamOrder) == 0 {
		return verb
	}
	parts := make([]string, 0, len(pathParamOrder)+1)
	parts = append(parts, verb)
	for _, param := range pathParamOrder {
		parts = append(parts, "<"+param+">")
	}
	return strings.Join(parts, " ")
}

func findOperationContext(operationByCommand map[*cobra.Command]*spec.Operation, cmd *cobra.Command) *spec.Operation {
	for current := cmd; current != nil; current = current.Parent() {
		if op, ok := operationByCommand[current]; ok {
			return op
		}
	}
	return nil
}
