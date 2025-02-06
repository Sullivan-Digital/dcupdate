package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
)

func readOneOf(files ...string) ([]byte, error) {
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			return os.ReadFile(file)
		}
	}

	return nil, os.ErrNotExist
}

func runCommandAndLogOutput(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if verbose {
				log.Printf("[stdout] %s", scanner.Text())
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if verbose {
				log.Printf("[stderr] %s", scanner.Text())
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	return nil
}
