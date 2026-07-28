package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/llm"
)

// handleModelsCmd dispatches the "models" subcommand.
func handleModelsCmd(args []string) {
	if len(args) == 0 {
		printModelsUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		modelsList(args[1:])
	case "delete":
		modelsDelete(args[1:])
	default:
		printModelsUsage()
		os.Exit(1)
	}
}

func printModelsUsage() {
	fmt.Println("Usage: cleanup-tool models <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  list    List installed LLM models across registries")
	fmt.Println("  delete  Delete a model from a registry")
}

func modelsList(args []string) {
	flagSet := flag.NewFlagSet("models list", flag.ContinueOnError)
	if err := flagSet.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "models list: %v\n", err)
		os.Exit(1)
	}

	client := llm.NewClient()
	registries, err := client.GetRegistries(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "models list: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REGISTRY\tMODEL\tSIZE\t")
	for _, reg := range registries {
		if len(reg.Models) == 0 {
			fmt.Fprintf(w, "%s\t(no models)\t\n", reg.Name)
			continue
		}
		for _, m := range reg.Models {
			fmt.Fprintf(w, "%s\t%s\t%s\t\n", reg.Name, m.Name, analyzer.PrettySize(m.Size))
		}
	}
	_ = w.Flush()
}

func modelsDelete(args []string) {
	flagSet := flag.NewFlagSet("models delete", flag.ContinueOnError)
	if err := flagSet.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "models delete: %v\n", err)
		os.Exit(1)
	}
	positionals := flagSet.Args()
	if len(positionals) != 2 {
		fmt.Fprintln(os.Stderr, "models delete: expected <registry-name> <model-name>")
		os.Exit(1)
	}

	client := llm.NewClient()
	if err := client.DeleteModel(context.Background(), positionals[0], positionals[1]); err != nil {
		fmt.Fprintf(os.Stderr, "models delete: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Deleted %s from %s\n", positionals[1], positionals[0])
}
