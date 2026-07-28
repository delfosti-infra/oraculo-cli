package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type flowOwnership int

const (
	ownershipMine flowOwnership = iota
	ownershipOrphan
	ownershipForeign
)

type flowSelection struct {
	Push      []string
	Adopted   []string
	Foreign   []foreignFlow
	Unchanged []unchangedFlow
}

type foreignFlow struct {
	Slug        string
	ProjectName string
}

type unchangedFlow struct {
	Slug     string
	PushedAt time.Time
}

func classifyFlowOwnership(meta flowMeta, projectRefId string) flowOwnership {
	if strings.TrimSpace(meta.ProjectRefId) == "" {
		return ownershipOrphan
	}
	if meta.ProjectRefId == projectRefId {
		return ownershipMine
	}
	return ownershipForeign
}

func specFingerprint(spec string) string {
	sum := sha256.Sum256([]byte(normalizeSpecForFingerprint(spec)))
	return hex.EncodeToString(sum[:])
}

func normalizeSpecForFingerprint(spec string) string {
	normalized := strings.ReplaceAll(spec, "\r\n", "\n")
	return strings.TrimRight(normalized, "\n \t")
}

func isAlreadyPushed(meta flowMeta, spec string) bool {
	if meta.SpecHash == "" {
		return false
	}
	return meta.SpecHash == specFingerprint(spec)
}

func describeProjectOwner(meta flowMeta) string {
	if name := strings.TrimSpace(meta.ProjectName); name != "" {
		return name
	}
	if refId := strings.TrimSpace(meta.ProjectRefId); refId != "" {
		return fmt.Sprintf("proyecto %s", refId)
	}
	return "otro proyecto"
}

func groupForeignByProject(foreign []foreignFlow) []string {
	byProject := map[string][]string{}
	for _, f := range foreign {
		byProject[f.ProjectName] = append(byProject[f.ProjectName], f.Slug)
	}
	lines := make([]string, 0, len(byProject))
	for project, slugs := range byProject {
		sort.Strings(slugs)
		lines = append(lines, fmt.Sprintf("%s: %s", project, strings.Join(slugs, ", ")))
	}
	sort.Strings(lines)
	return lines
}
