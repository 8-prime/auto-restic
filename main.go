package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
)

func main() {
	// Setup logging
	log.SetPrefix("[restic] ")
	log.SetFlags(log.Ldate | log.Ltime)

	// Validate environment variables
	if err := validateEnv(); err != nil {
		log.Fatal(err)
	}

	// Setup SSH keys
	if err := setupSSH(); err != nil {
		log.Fatal(err)
	}

	// Check or initialize repository
	if err := checkOrInitRepository(); err != nil {
		log.Fatal(err)
	}

	// Setup cron scheduler
	cronSchedule := os.Getenv("CRON_SCHEDULE")
	log.Printf("Scheduling backups with: %s", cronSchedule)

	c := cron.New()
	_, err := c.AddFunc(cronSchedule, func() {
		if err := runBackup(); err != nil {
			log.Printf("Backup failed: %v", err)
		} else {
			log.Println("Backup completed successfully")
		}
	})
	if err != nil {
		log.Fatalf("Invalid cron schedule: %v", err)
	}

	c.Start()
	log.Println("Scheduler started, waiting for scheduled runs...")

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	<-sigChan
	log.Println("Received shutdown signal, stopping scheduler...")
	c.Stop()
	log.Println("Shutdown complete")
}

func validateEnv() error {
	required := []string{"RESTIC_REPOSITORY", "RESTIC_PASSWORD", "BACKUP_PATH", "CRON_SCHEDULE"}
	var missing []string

	for _, v := range required {
		if os.Getenv(v) == "" {
			missing = append(missing, v)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return nil
}

func setupSSH() error {
	sshKeysSource := "/keys"
	sshDir := "/root/.ssh"

	if _, err := os.Stat(sshKeysSource); os.IsNotExist(err) {
		log.Printf("Warning: No SSH keys directory found at %s", sshKeysSource)
		return nil
	}

	log.Println("Setting up SSH configuration...")

	// Create SSH directory
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create SSH directory: %w", err)
	}

	// Copy SSH keys
	entries, err := os.ReadDir(sshKeysSource)
	if err != nil {
		return fmt.Errorf("failed to read SSH keys directory: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(sshKeysSource, entry.Name())
		dstPath := filepath.Join(sshDir, entry.Name())

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", srcPath, err)
		}

		mode := os.FileMode(0600)
		if entry.Name() == "config" {
			mode = 0644
		}

		if err := os.WriteFile(dstPath, data, mode); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstPath, err)
		}
	}

	// Set environment variable for restic
	os.Setenv("RESTIC_SSH_COMMAND", "ssh -F /root/.ssh/config")

	if _, err := os.Stat(filepath.Join(sshDir, "config")); err == nil {
		log.Println("SSH config found")
	}

	log.Println("SSH keys configured")
	return nil
}

func checkOrInitRepository() error {
	repo := os.Getenv("RESTIC_REPOSITORY")

	log.Println("Checking repository…")

	cmd := exec.Command("restic", "snapshots", "-r", repo)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		log.Println("Repository not found, initializing…")

		cmd = exec.Command("restic", "init", "-r", repo)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to initialize repository: %w", err)
		}

		log.Println("Performing first backup…")
		return runBackup()
	}

	log.Println("Repository exists")
	return nil
}

func runBackup() error {
	repo := os.Getenv("RESTIC_REPOSITORY")
	backupPath := os.Getenv("BACKUP_PATH")
	resticArgs := os.Getenv("RESTIC_ARGS")

	log.Printf("Starting backup at %s", time.Now().Format("2006-01-02 15:04:05"))

	args := []string{"backup", backupPath, "-r", repo}
	if resticArgs != "" {
		args = append(args, strings.Fields(resticArgs)...)
	}

	cmd := exec.Command("restic", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("backup command failed: %w", err)
	}

	return nil
}
