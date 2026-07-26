package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"redis-proxy/config"
	"redis-proxy/server"
)

const pidFilePath = "redis-proxy.pid"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "start":
		handleStart()
	case "stop":
		handleStop()
	case "reload":
		handleReload()
	case "check":
		handleCheck()
	case "help", "-h", "--help":
		printUsage()
	default:
		log.Printf("Unknown command: %s", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: redis-proxy <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  start     Start the redis proxy server")
	fmt.Println("            Options:")
	fmt.Println("              -config <path>   Path to configuration file (default: redis-proxy.conf)")
	fmt.Println("              -daemon          Run the proxy in background (daemon mode)")
	fmt.Println("              -verbose         Enable verbose logging (hostname/backends)")
	fmt.Println("  stop      Stop the running proxy daemon")
	fmt.Println("  reload    Reload the running proxy configuration (zero-downtime)")
	fmt.Println("  check     Verify configuration file validity")
	fmt.Println("            Options:")
	fmt.Println("              -config <path>   Path to configuration file (default: redis-proxy.conf)")
	fmt.Println("  help      Show this help message")
}

func getRunningProcess() (*os.Process, error) {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		return nil, err
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return nil, err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	// On Unix, FindProcess always returns a process object; we must send signal 0
	// to see if the process actually exists and is active.
	err = proc.Signal(syscall.Signal(0))
	if err != nil {
		return nil, err
	}
	return proc, nil
}

func handleCheck() {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", "redis-proxy.conf", "Path to configuration file")
	_ = fs.Parse(os.Args[2:])

	_, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("Configuration is INVALID: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Configuration in %s is VALID\n", *configPath)
	os.Exit(0)
}

func handleStart() {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "redis-proxy.conf", "Path to configuration file")
	daemon := fs.Bool("daemon", false, "Start in background (daemon mode)")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")
	// Hidden flag used internally when spawning the daemon subprocess
	internalDaemon := fs.Bool("internal-daemon", false, "Internal daemon flag")
	_ = fs.Parse(os.Args[2:])

	// Check if already running
	proc, err := getRunningProcess()
	if err == nil && proc != nil {
		log.Fatalf("redis-proxy is already running (PID: %d)", proc.Pid)
	}

	if *daemon && !*internalDaemon {
		err := spawnDaemon(*configPath, *verbose)
		if err != nil {
			log.Fatalf("Failed to start daemon: %v", err)
		}
		return
	}

	// Write PID file
	err = os.WriteFile(pidFilePath, []byte(strconv.Itoa(os.Getpid())), 0644)
	if err != nil {
		log.Fatalf("Failed to write PID file: %v", err)
	}

	// Ensure PID file is cleaned up if we exit abnormally
	defer os.Remove(pidFilePath)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	isVerbose := cfg.Verbose || *verbose
	log.Printf("Loaded configuration from: %s (verbose=%t)", *configPath, isVerbose)
	srv, err := server.NewServer(cfg.ListenAddr, cfg.Backends, cfg.LoadBalance, isVerbose)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Setup signal channels
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Printf("Received signal %s, shutting down...", sig)
				os.Remove(pidFilePath)
				os.Exit(0)
			case syscall.SIGHUP:
				log.Printf("Received SIGHUP, reloading configuration...")
				newCfg, err := config.LoadConfig(*configPath)
				if err != nil {
					log.Printf("Failed to load configuration on reload: %v", err)
					continue
				}
				isVerboseReload := newCfg.Verbose || *verbose
				err = srv.Reload(newCfg.Backends, newCfg.LoadBalance, isVerboseReload)
				if err != nil {
					log.Printf("Failed to reload server: %v", err)
				}
			}
		}
	}()

	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func spawnDaemon(configPath string, verbose bool) error {
	// Re-execute binary with start --config <path> --internal-daemon
	args := []string{"start", "-config", configPath, "-internal-daemon"}
	if verbose {
		args = append(args, "-verbose")
	}
	cmd := exec.Command(os.Args[0], args...)

	// Open log file to redirect stdout/stderr
	logFile, err := os.OpenFile("redis-proxy.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil

	// Detach child process from current session
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	fmt.Printf("Started redis-proxy in background (PID: %d, logs: redis-proxy.log)\n", cmd.Process.Pid)
	return nil
}

func handleStop() {
	proc, err := getRunningProcess()
	if err != nil || proc == nil {
		log.Printf("redis-proxy is not running")
		return
	}

	log.Printf("Stopping redis-proxy (PID: %d)...", proc.Pid)
	err = proc.Signal(syscall.SIGTERM)
	if err != nil {
		log.Fatalf("Failed to send stop signal: %v", err)
	}

	// Poll up to 5 seconds to verify it exited and deleted the PID file
	for i := 0; i < 50; i++ {
		_, err := getRunningProcess()
		if err != nil {
			fmt.Println("redis-proxy stopped successfully.")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Process %d did not terminate gracefully. Killing it...", proc.Pid)
	_ = proc.Signal(syscall.SIGKILL)
	os.Remove(pidFilePath)
}

func handleReload() {
	proc, err := getRunningProcess()
	if err != nil || proc == nil {
		log.Fatalf("redis-proxy is not running (cannot reload)")
	}

	log.Printf("Reloading redis-proxy (PID: %d)...", proc.Pid)
	err = proc.Signal(syscall.SIGHUP)
	if err != nil {
		log.Fatalf("Failed to send reload signal: %v", err)
	}
	fmt.Println("Configuration reload signal sent successfully.")
}
