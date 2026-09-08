package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

type blockKey struct {
	File  string
	Start string
}

type block struct {
	File  string
	Start string
	NStmt int
	Count uint64
}

type profile struct {
	Mode   string
	Blocks map[blockKey]block
}

func parseProfile(path string) (profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return profile{}, fmt.Errorf("open profile %s: %w", path, err)
	}
	defer f.Close()
	return parseProfileReader(path, bufio.NewScanner(f))
}

func parseProfileReader(origin string, scanner *bufio.Scanner) (profile, error) {
	p := profile{Blocks: map[blockKey]block{}}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if lineNo == 1 || p.Mode == "" {
			mode, ok := strings.CutPrefix(line, "mode: ")
			if !ok || strings.TrimSpace(mode) == "" {
				return profile{}, fmt.Errorf("%s:%d: missing coverage mode", origin, lineNo)
			}
			p.Mode = strings.TrimSpace(mode)
			continue
		}
		blk, err := parseBlockLine(line)
		if err != nil {
			return profile{}, fmt.Errorf("%s:%d: %w", origin, lineNo, err)
		}
		key := blockKey{File: blk.File, Start: blk.Start}
		if existing, ok := p.Blocks[key]; ok {
			if existing.NStmt != blk.NStmt {
				return profile{}, fmt.Errorf("%s:%d: statement count mismatch for %s:%s", origin, lineNo, blk.File, blk.Start)
			}
			sum, err := addCounts(existing.Count, blk.Count)
			if err != nil {
				return profile{}, fmt.Errorf("%s:%d: %w", origin, lineNo, err)
			}
			blk.Count = sum
		}
		p.Blocks[key] = blk
	}
	if err := scanner.Err(); err != nil {
		return profile{}, fmt.Errorf("read profile %s: %w", origin, err)
	}
	if p.Mode == "" {
		return profile{}, fmt.Errorf("%s: empty or malformed coverage profile", origin)
	}
	if len(p.Blocks) == 0 {
		return profile{}, fmt.Errorf("%s: coverage profile has no blocks", origin)
	}
	return p, nil
}

func parseBlockLine(line string) (block, error) {
	file, rest, ok := strings.Cut(line, ":")
	if !ok || file == "" || rest == "" {
		return block{}, fmt.Errorf("malformed coverage block %q", line)
	}
	start, nstmtCount, ok := strings.Cut(rest, " ")
	if !ok {
		return block{}, fmt.Errorf("malformed coverage block %q", line)
	}
	nstmtRaw, countRaw, ok := strings.Cut(nstmtCount, " ")
	if !ok {
		return block{}, fmt.Errorf("malformed coverage block %q", line)
	}
	if strings.ContainsAny(start, " \t") || !strings.Contains(start, ",") {
		return block{}, fmt.Errorf("malformed coverage locations %q", start)
	}
	nstmt, err := strconv.Atoi(nstmtRaw)
	if err != nil || nstmt < 0 {
		return profileBlockErr("invalid statement count", nstmtRaw)
	}
	count, err := strconv.ParseUint(countRaw, 10, 64)
	if err != nil {
		return profileBlockErr("invalid execution count", countRaw)
	}
	return block{File: file, Start: start, NStmt: nstmt, Count: count}, nil
}

func profileBlockErr(msg, raw string) (block, error) {
	return block{}, fmt.Errorf("%s %q", msg, raw)
}

func addCounts(a, b uint64) (uint64, error) {
	if b > math.MaxUint64-a {
		return 0, fmt.Errorf("coverage count overflow")
	}
	return a + b, nil
}

func mergeChildIntoParent(parent profile, child profile, scopePrefix string) (profile, int, error) {
	if parent.Mode == "" || len(parent.Blocks) == 0 {
		return profile{}, 0, fmt.Errorf("parent coverage profile is empty")
	}
	if child.Mode == "" {
		return profile{}, 0, fmt.Errorf("child coverage profile is empty")
	}
	if parent.Mode != child.Mode {
		return profile{}, 0, fmt.Errorf("coverage mode mismatch: parent %s child %s", parent.Mode, child.Mode)
	}
	added := 0
	for key, childBlk := range child.Blocks {
		if scopePrefix != "" && !strings.HasPrefix(childBlk.File, scopePrefix) {
			continue
		}
		parentBlk, ok := parent.Blocks[key]
		if !ok {
			if scopePrefix != "" && strings.HasPrefix(childBlk.File, scopePrefix) {
				return profile{}, 0, fmt.Errorf("unknown in-scope child block %s:%s", childBlk.File, childBlk.Start)
			}
			return profile{}, 0, fmt.Errorf("unknown child block %s:%s", childBlk.File, childBlk.Start)
		}
		if parentBlk.NStmt != childBlk.NStmt {
			return profile{}, 0, fmt.Errorf("statement count mismatch for %s:%s parent=%d child=%d", childBlk.File, childBlk.Start, parentBlk.NStmt, childBlk.NStmt)
		}
		sum, err := addCounts(parentBlk.Count, childBlk.Count)
		if err != nil {
			return profile{}, 0, err
		}
		if parentBlk.Count == 0 && sum > 0 {
			added += parentBlk.NStmt
		}
		parentBlk.Count = sum
		parent.Blocks[key] = parentBlk
	}
	return parent, added, nil
}

func profileTotals(p profile) (total, covered int) {
	for _, blk := range p.Blocks {
		total += blk.NStmt
		if blk.Count > 0 {
			covered += blk.NStmt
		}
	}
	return total, covered
}

func formatPercent(covered, total int) string {
	if total == 0 {
		return "0.0"
	}
	return fmt.Sprintf("%.1f", 100*float64(covered)/float64(total))
}

func writeProfile(path string, p profile) error {
	keys := make([]blockKey, 0, len(p.Blocks))
	for key := range p.Blocks {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].File != keys[j].File {
			return keys[i].File < keys[j].File
		}
		return keys[i].Start < keys[j].Start
	})
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = fmt.Fprintf(f, "mode: %s\n", p.Mode); err != nil {
		return err
	}
	for _, key := range keys {
		blk := p.Blocks[key]
		if _, err = fmt.Fprintf(f, "%s:%s %d %d\n", blk.File, blk.Start, blk.NStmt, blk.Count); err != nil {
			return err
		}
	}
	return f.Close()
}
