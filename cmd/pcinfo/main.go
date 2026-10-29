// pcinfo — shu PC haqida htop'ga o'xshash terminal dasturi.
//
//	pcinfo                 TUI (odatda shranet nomidan)
//	pcinfo collect --json  claude tomoni: rootless Docker va proyektlar (sudo orqali)
//
// Dastur faqat o'qiydi: hech qanday konteyner, fayl yoki sozlamani o'zgartirmaydi.
// Yagona tashqi "yozish" — modemga balans USSD'si (*107#).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/shranet/pcinfo/internal/collect/modem"
	"github.com/shranet/pcinfo/internal/collect/process"
	"github.com/shranet/pcinfo/internal/collect/system"
	"github.com/shranet/pcinfo/internal/privileged"
	"github.com/shranet/pcinfo/internal/snapshot"
	"github.com/shranet/pcinfo/internal/tui"
)

// version build paytida -ldflags "-X main.version=…" bilan beriladi.
var version = "dev"

const (
	collectCommand = "collect"
	collectJSONArg = "--json"
	collectTimeout = 30 * time.Second

	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == collectCommand {
		return runCollect(args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("pcinfo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	publicIP := fs.Bool("public-ip", false, "har liniyaning tashqi IP manzilini so'rash (api.ipify.org ga so'rov ketadi)")
	noUSSD := fs.Bool("no-ussd", false, "ishga tushganda balans USSD'sini yubormaslik")
	showVersion := fs.Bool("version", false, "versiyani chiqarish")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "noma'lum argument: %q\n", fs.Arg(0))
		return exitUsage
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return exitOK
	}
	return runTUI(tui.Options{PublicIP: *publicIP, USSD: !*noUSSD, Version: version}, stderr)
}

func runTUI(opts tui.Options, stderr io.Writer) int {
	deps := tui.Deps{
		Claude:       privileged.NewFetcher(),
		Modem:        modem.New(modem.DefaultBaseURL),
		System:       system.NewSampler(),
		Process:      process.NewSampler(),
		SelfWarnings: privileged.SelfCheck(),
	}
	if _, err := tea.NewProgram(tui.New(deps, opts)).Run(); err != nil {
		fmt.Fprintln(stderr, "pcinfo:", err)
		return exitError
	}
	return exitOK
}

// runCollect — sudoers faqat aynan `collect --json` ga ruxsat beradi (D-003),
// shuning uchun boshqa hech qanday argument qabul qilinmaydi.
func runCollect(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != collectJSONArg {
		fmt.Fprintf(stderr, "foydalanish: pcinfo %s %s\n", collectCommand, collectJSONArg)
		return exitUsage
	}
	ctx, cancel := context.WithTimeout(context.Background(), collectTimeout)
	defer cancel()
	if err := json.NewEncoder(stdout).Encode(snapshot.CollectClaudeSide(ctx)); err != nil {
		fmt.Fprintln(stderr, "pcinfo collect:", err)
		return exitError
	}
	return exitOK
}
