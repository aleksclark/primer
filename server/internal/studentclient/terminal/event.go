package terminal

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal/observe"
)

// BuildStructuredEvent constructs a ShellEvent from a trusted process-wait result.
func BuildStructuredEvent(
	seq int64,
	sessionID string,
	cwdBefore, cwdAfterRel string,
	executable string,
	argv []string,
	submitted string,
	exitCode int,
	stdout, stderr string,
	runnerVer, verifierVer string,
	manifestBefore, manifestAfter contracts.WorkspaceManifest,
) contracts.ShellEvent {
	q := contracts.EvidenceQuality{
		Exit:   true,
		Cwd:    cwdAfterRel != "",
		Argv:   executable != "" || len(argv) > 0,
		Stdout: true,
		Stderr: true,
	}
	exe := executable
	args := append([]string(nil), argv...)
	argvOK := q.Argv
	var pipe *contracts.PipelineInfo
	if submitted != "" {
		if pe, pa, ok, p := observe.ParseCommandLine(submitted); ok {
			if exe == "/bin/sh" || exe == "sh" || filepath.Base(exe) == "sh" {
				if len(args) >= 2 && args[0] == "-c" {
					exe = pe
					args = pa
					argvOK = true
					pipe = p
				}
			} else if exe == "" {
				exe = pe
				args = pa
				argvOK = ok
				pipe = p
			} else {
				pipe = p
			}
		}
	}
	q.Argv = argvOK
	writeSet := WriteSetDiff(manifestBefore, manifestAfter)
	return contracts.ShellEvent{
		SchemaVersion:        contracts.ShellEventSchemaVersion,
		SessionID:            sessionID,
		Sequence:             seq,
		FinishedAt:           time.Now().UTC(),
		SubmittedLine:        truncateStr(submitted, observe.MaxSubmittedLine),
		Executable:           exe,
		Argv:                 args,
		ArgvAvailable:        argvOK,
		CwdBefore:            cwdBefore,
		CwdAfter:             cwdAfterRel,
		CwdAvailable:         q.Cwd,
		ExitCode:             exitCode,
		ExitAvailable:        true,
		Stdout:               observe.BoundExcerpt(stdout, true),
		Stderr:               observe.BoundExcerpt(stderr, true),
		Pipeline:             pipe,
		ManifestBefore:       manifestBefore.Digest,
		ManifestAfter:        manifestAfter.Digest,
		WriteSet:             writeSet,
		RunnerVersion:        runnerVer,
		ShellInstrumentation: "process-wait/1",
		VerifierVersion:      verifierVer,
		Source:               contracts.SourceStructured,
		Structured:           q.MeetsStructuredBar(),
		Quality:              q,
	}
}

// DigestBytes returns hex sha256.
func DigestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func truncateStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
