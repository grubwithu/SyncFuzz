package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/coverage"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

func recoveryRelationNovelty(args []string) {
	fs := flag.NewFlagSet("recovery relation-novelty", flag.ExitOnError)
	reportsValue := fs.String("reports", "", "comma-separated recovery relation report JSON paths")
	fidelityBatchRoot := fs.String("fidelity-batch", "", "fidelity batch root; loads accepted full/recovery-relation-report.json artifacts")
	ledgerPath := fs.String("ledger", "", "optional existing RelationNoveltyLedger JSON path")
	outPath := fs.String("out", "relation-novelty-ledger.json", "RelationNoveltyLedger JSON output path")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	hasReports := strings.TrimSpace(*reportsValue) != ""
	hasBatch := strings.TrimSpace(*fidelityBatchRoot) != ""
	if hasReports == hasBatch || strings.TrimSpace(*outPath) == "" {
		fmt.Fprintln(os.Stderr, "syncfuzz recovery relation-novelty requires exactly one of --reports or --fidelity-batch and --out")
		os.Exit(2)
	}

	reports := make([]recovery.RecoveryRelationReport, 0)
	loadReport := func(path string) error {
		report, err := recovery.ReadRecoveryRelationReport(path)
		if err != nil {
			return err
		}
		reports = append(reports, report)
		return nil
	}
	if hasReports {
		for _, path := range splitCSV(*reportsValue) {
			if err := loadReport(path); err != nil {
				fmt.Fprintf(os.Stderr, "syncfuzz recovery relation-novelty failed: %v\n", err)
				os.Exit(1)
			}
		}
	} else {
		entries, err := os.ReadDir(*fidelityBatchRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "syncfuzz recovery relation-novelty failed: read fidelity batch root %s: %v\n", *fidelityBatchRoot, err)
			os.Exit(1)
		}
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "attempt-") {
				continue
			}
			path := filepath.Join(*fidelityBatchRoot, entry.Name(), "full", "recovery-relation-report.json")
			if err := loadReport(path); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				fmt.Fprintf(os.Stderr, "syncfuzz recovery relation-novelty failed: %v\n", err)
				os.Exit(1)
			}
		}
	}
	if len(reports) == 0 {
		fmt.Fprintln(os.Stderr, "syncfuzz recovery relation-novelty failed: no complete full recovery relation reports were found")
		os.Exit(1)
	}

	ledger := coverage.RelationNoveltyLedger{}
	if strings.TrimSpace(*ledgerPath) != "" {
		value, err := coverage.ReadRelationNoveltyLedger(*ledgerPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "syncfuzz recovery relation-novelty failed: %v\n", err)
			os.Exit(1)
		}
		ledger = value
	}
	updated, summary, err := coverage.UpdateRelationNoveltyLedger(ledger, reports)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz recovery relation-novelty failed: %v\n", err)
		os.Exit(1)
	}
	if err := coverage.WriteRelationNoveltyLedger(*outPath, updated); err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz recovery relation-novelty failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("input_reports: %d\n", summary.InputReportCount)
	fmt.Printf("added_records: %d\n", summary.AddedRecordCount)
	fmt.Printf("records: %d\n", summary.RecordCount)
	fmt.Printf("unique_relation_tuples: %d\n", summary.UniqueTupleCount)
	fmt.Printf("proven_causal_records: %d\n", summary.ProvenCausalRecordCount)
	fmt.Printf("unknown_causal_records: %d\n", summary.UnknownCausalRecordCount)
	fmt.Printf("artifact: %s\n", *outPath)
}
