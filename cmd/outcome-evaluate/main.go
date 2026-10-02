// outcome-evaluate is a read-only bootstrap evidence tool, not an execution
// controller. A disposition never invokes a task or grants effect authority.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/openexec/openexec/pkg/outcome"
)

func main() {
	input := flag.String("input", "", "bounded authoritative snapshot JSON")
	output := flag.String("output", "", "write a bootstrap evaluation receipt")
	flag.Parse()
	if err := run(*input, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(input, output string) error {
	if input == "" || output == "" {
		return fmt.Errorf("--input and --output required")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	if len(raw) > 64000 {
		return fmt.Errorf("snapshot exceeds bounded input")
	}
	var in outcome.Input
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}
	e, err := outcome.Evaluate(context.Background(), nil, in)
	if err != nil {
		return err
	}
	receipt := struct {
		At          string             `json:"at"`
		InputSHA256 string             `json:"input_sha256"`
		Evaluation  outcome.Evaluation `json:"evaluation"`
		Limitation  string             `json:"limitation"`
	}{time.Now().UTC().Format(time.RFC3339Nano), fmt.Sprintf("%x", sha256.Sum256(raw)), e, "Bootstrap invocation only. This read-only command proves classification of the supplied snapshot, not autonomous recovery or product Ready."}
	b, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(output, append(b, '\n'), 0600); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
