// Command foc runs Fog of Command as a local web game, or evaluates a
// decision provider with `foc eval`.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/deosjr/foc/internal/config"
	"github.com/deosjr/foc/internal/game"
	"github.com/deosjr/foc/internal/replay"
	"github.com/deosjr/foc/internal/runlog"
	"github.com/deosjr/foc/internal/web"
	"github.com/deosjr/foc/internal/wiring"

	// Providers register themselves; adding one is one import line.
	_ "github.com/deosjr/foc/internal/providers/decision/llm"
	_ "github.com/deosjr/foc/internal/providers/decision/mock"
	_ "github.com/deosjr/foc/internal/providers/llm/anthropic"
	_ "github.com/deosjr/foc/internal/providers/llm/claudecli"
	_ "github.com/deosjr/foc/internal/providers/llm/mock"
	_ "github.com/deosjr/foc/internal/providers/llm/openaicompat"
)

func main() {
	if len(os.Args) > 2 && os.Args[1] == "replay" {
		os.Exit(runReplay(os.Args[2]))
	}
	if len(os.Args) > 1 && os.Args[1] == "eval" {
		if err := runEval(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "foc eval:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "foc:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "config file (default: config.yaml if present, else built-in mock defaults)")
	seed := flag.Uint64("seed", 0, "random seed (overrides config)")
	debug := flag.Bool("debug", false, "show the debug drawer and the /debug/truth page")
	replay := flag.String("replay", "", "replay a saved run directory and check it reproduces its turns.jsonl")
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	open := flag.Bool("open", false, "open the browser on start")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if *seed != 0 {
		cfg.Seed = *seed
	}
	if *replay != "" {
		os.Exit(runReplay(*replay))
	}
	runDir := filepath.Join(cfg.RunsDir, fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), cfg.Seed))
	tlog := runlog.New()
	dm, lm, recorder, err := wiring.Models(cfg, tlog)
	if err != nil {
		return err
	}
	g, err := game.New(game.Options{Config: cfg, Decision: dm, LLM: lm, Log: tlog, RunDir: runDir, Recorder: recorder})
	if err != nil {
		return err
	}
	srv, err := web.New(g, *debug)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()
	log.Printf("Fog of Command: %s (decision: %s, llm: %s, seed %d%s)", url,
		cfg.Decision.Provider, cfg.LLM.Provider, cfg.Seed, map[bool]string{true: ", debug", false: ""}[*debug])
	if *open {
		openBrowser(url)
	}
	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shutdown)
	}()
	if err := hs.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// loadConfig uses the given path, else config.yaml if it exists, else the
// built-in offline defaults.
func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			path = "config.yaml"
		}
	}
	return config.Load(path)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open browser: %v", err)
	}
}

// runReplay replays a run directory and reports whether it reproduced the
// original log. It returns the process exit code.
func runReplay(dir string) int {
	res, err := replay.Run(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "foc replay:", err)
		return 1
	}
	if res.Identical {
		fmt.Printf("Replayed %d turns of %s: turns.jsonl reproduced exactly (ignoring latency).\n", res.Turns, dir)
		return 0
	}
	fmt.Printf("Replayed %d turns of %s: logs differ at line %d.\n  original: %.300s\n  replayed: %.300s\nFull replay log: %s\n",
		res.Turns, dir, res.Line, res.Original, res.Replayed, res.Output)
	return 2
}
