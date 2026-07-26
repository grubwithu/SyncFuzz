package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
)

func runEnvironment(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "syncfuzz environment requires a subcommand; supported: unix-socket-program")
		os.Exit(2)
	}
	switch args[0] {
	case "unix-socket-program":
		environmentUnixSocketProgram(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown environment subcommand: %s\n", args[0])
		os.Exit(2)
	}
}

// environmentUnixSocketProgram creates a fully validated E artifact. When
// roles differ it emits an explicit rebind mutation whose parent is the
// corresponding baseline topology; callers never have to forge ProgramID.
func environmentUnixSocketProgram(args []string) {
	fs := flag.NewFlagSet("environment unix-socket-program", flag.ExitOnError)
	out := fs.String("out", "", "output EnvironmentProgram JSON path")
	logicalName := fs.String("logical-name", "", "logical service name")
	resolutionMode := fs.String("resolution-mode", "", "direct, config, environment, or alias")
	resolutionKey := fs.String("resolution-key", "", "config/environment/alias lookup key")
	resolutionArtifact := fs.String("resolution-artifact-path", "", "workspace-relative config or alias artifact")
	endpointPath := fs.String("endpoint-path", "", "workspace-relative Unix socket endpoint")
	initialRole := fs.String("initial-role", "", "semantic role before a possible rebind")
	activeRole := fs.String("active-role", "", "semantic role of the active endpoint")
	holderLifetime := fs.String("holder-lifetime", "child", "child, foreground, or detached")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if strings.TrimSpace(*out) == "" || strings.TrimSpace(*logicalName) == "" || strings.TrimSpace(*resolutionMode) == "" || strings.TrimSpace(*endpointPath) == "" || strings.TrimSpace(*initialRole) == "" || strings.TrimSpace(*activeRole) == "" {
		fmt.Fprintln(os.Stderr, "syncfuzz environment unix-socket-program requires --out, --logical-name, --resolution-mode, --endpoint-path, --initial-role, and --active-role")
		os.Exit(2)
	}
	mode := environment.UnixSocketResolutionMode(strings.TrimSpace(*resolutionMode))
	lifetime := environment.HolderLifetime(strings.TrimSpace(*holderLifetime))
	baseline, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName:            *logicalName,
		ResolutionMode:         mode,
		ResolutionKey:          *resolutionKey,
		ResolutionArtifactPath: *resolutionArtifact,
		EndpointPath:           *endpointPath,
		InitialRole:            *initialRole,
		ActiveRole:             *initialRole,
		HolderLifetime:         lifetime,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz environment unix-socket-program failed: %v\n", err)
		os.Exit(1)
	}
	program := baseline
	if strings.TrimSpace(*activeRole) != strings.TrimSpace(*initialRole) {
		program, err = baseline.MutateUnixSocket(environment.UnixSocketMutation{
			Operator:   environment.MutationOperatorRebind,
			ActiveRole: *activeRole,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "syncfuzz environment unix-socket-program failed: %v\n", err)
			os.Exit(1)
		}
	}
	if err := environment.WriteEnvironmentProgram(*out, program); err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz environment unix-socket-program failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("program_id: %s\n", program.ProgramID)
	fmt.Printf("mutation_operator: %s\n", program.Mutation.Operator)
	fmt.Printf("artifact: %s\n", *out)
}
