package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"strings"

	"plenilune/internal/analysis"
	"plenilune/internal/jsonio"
)

const maxRequestBytes = 2*analysis.MaxSourceBytes + 2<<20

// replayRequest carries original text and the artifact in one JSON document
// so an integrating caller never has to write decrypted configuration to disk.
type replayRequest struct {
	Schema   string          `json:"schema"`
	Before   analysis.Source `json:"before"`
	After    analysis.Source `json:"after"`
	Evidence analysis.Result `json:"evidence"`
}

func readInput(ctx context.Context, path string, stdin io.Reader, limit int) ([]byte, error) {
	if path != "-" {
		return read(ctx, path, limit)
	}
	data, err := io.ReadAll(io.LimitReader(stdin, int64(limit)+1))
	if err != nil {
		return nil, errors.New("cannot read standard input")
	}
	if len(data) > limit {
		return nil, errors.New("input exceeds size limit")
	}
	return data, nil
}

func analysisFailure(stderr io.Writer, err error) int {
	var e *analysis.Error
	if errors.As(err, &e) {
		return fail(stderr, e.Code, e.Message)
	}
	return fail(stderr, "ANALYSIS_FAILED", "analysis failed")
}

func runAnalysis(ctx context.Context, cmd string, args []string, stdin io.Reader, out, stderr io.Writer) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var requestPath, beforePath, afterPath, evidencePath, selectList string
	fs.StringVar(&requestPath, "request", "", "request JSON file, or - for standard input")
	fs.StringVar(&beforePath, "before", "", "before configuration file")
	fs.StringVar(&afterPath, "after", "", "after configuration file")
	if cmd == "analyze" {
		fs.StringVar(&selectList, "select", "", "comma-separated domain:name objects to analyze (default all)")
	} else {
		fs.StringVar(&evidencePath, "evidence", "", "analysis artifact JSON")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(out)
			fs.PrintDefaults()
			return 0
		}
		return fail(stderr, "USAGE", "invalid analysis arguments")
	}
	usingFiles := beforePath != "" || afterPath != ""
	if fs.NArg() != 0 || usingFiles == (requestPath != "") || usingFiles && (beforePath == "" || afterPath == "") || cmd == "analyze-replay" && usingFiles && evidencePath == "" {
		return fail(stderr, "USAGE", "use --request, or --before and --after")
	}
	if cmd == "analyze" {
		var req analysis.Request
		if requestPath != "" {
			data, err := readInput(ctx, requestPath, stdin, maxRequestBytes)
			if err != nil {
				return fail(stderr, "INPUT_FAILED", err.Error())
			}
			if err := jsonio.Decode(data, maxRequestBytes, &req); err != nil {
				return fail(stderr, "INVALID_JSON", err.Error())
			}
		} else {
			before, err := read(ctx, beforePath, analysis.MaxSourceBytes)
			if err != nil {
				return fail(stderr, "INPUT_FAILED", err.Error())
			}
			after, err := read(ctx, afterPath, analysis.MaxSourceBytes)
			if err != nil {
				return fail(stderr, "INPUT_FAILED", err.Error())
			}
			req = analysis.Request{Schema: analysis.RequestSchema, Before: analysis.Source{Text: string(before)}, After: analysis.Source{Text: string(after)}}
			for _, item := range strings.Split(selectList, ",") {
				if item = strings.TrimSpace(item); item == "" {
					continue
				}
				domain, name, ok := strings.Cut(item, ":")
				if !ok {
					return fail(stderr, "USAGE", "select entries are domain:name")
				}
				req.Select = append(req.Select, analysis.ObjectRef{Domain: domain, Name: name})
			}
		}
		result, err := analysis.Analyze(ctx, req)
		if err != nil {
			return analysisFailure(stderr, err)
		}
		return output(out, stderr, result)
	}
	var before, after []byte
	var artifact analysis.Result
	if requestPath != "" {
		data, err := readInput(ctx, requestPath, stdin, maxRequestBytes+analysis.MaxResultBytes)
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		var req replayRequest
		if err := jsonio.Decode(data, maxRequestBytes+analysis.MaxResultBytes, &req); err != nil {
			return fail(stderr, "INVALID_JSON", err.Error())
		}
		if req.Schema != analysis.ReplayRequestSchema {
			return fail(stderr, "REQUEST_SCHEMA", "expected "+analysis.ReplayRequestSchema)
		}
		before, after, artifact = []byte(req.Before.Text), []byte(req.After.Text), req.Evidence
	} else {
		var err error
		if before, err = read(ctx, beforePath, analysis.MaxSourceBytes); err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		if after, err = read(ctx, afterPath, analysis.MaxSourceBytes); err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		data, err := read(ctx, evidencePath, analysis.MaxResultBytes)
		if err != nil {
			return fail(stderr, "INPUT_FAILED", err.Error())
		}
		if err := jsonio.Decode(data, analysis.MaxResultBytes, &artifact); err != nil {
			return fail(stderr, "INVALID_JSON", err.Error())
		}
	}
	result, err := analysis.Replay(ctx, before, after, artifact)
	if err != nil {
		return analysisFailure(stderr, err)
	}
	if code := output(out, stderr, result); code != 0 {
		return code
	}
	if result.Status == "TIMING_SENSITIVE" {
		return 2
	}
	return 0
}

var stdinReader io.Reader = os.Stdin
