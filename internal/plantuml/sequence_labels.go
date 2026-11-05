package plantuml

import (
	"path/filepath"
	"strings"

	"github.com/Mino829/umlgen/internal/sequence"
)

// Diagram labels omit repository context only when the short label is unique.
// Canonical participant IDs and the explanation retain the complete identity.
func sequenceParticipantLabels(participants []sequence.Participant) []string {
	labels := make([]string, len(participants))
	counts := make(map[string]int, len(participants))
	for n, participant := range participants {
		label := participant.Label
		if at := strings.LastIndex(label, "::"); at >= 0 {
			label = label[at+2:]
		}
		labels[n] = label
		counts[label]++
	}
	for n, label := range labels {
		if counts[label] > 1 {
			labels[n] = participants[n].Label
		}
	}
	return labels
}

func sequenceSourcePaths(files []sequence.File) map[string]string {
	paths := make(map[string]string, len(files))
	if len(files) == 0 {
		return paths
	}
	root := filepath.Dir(files[0].Path)
	for _, file := range files[1:] {
		for {
			relative, err := filepath.Rel(root, file.Path)
			if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				break
			}
			root = parent
		}
	}
	for _, file := range files {
		relative, err := filepath.Rel(root, file.Path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			paths[file.Path] = file.Path
		} else {
			paths[file.Path] = filepath.ToSlash(relative)
		}
	}
	return paths
}

func sequenceCandidateLabels(callables []sequence.Callable) map[string]string {
	counts := make(map[string]int, len(callables))
	for _, callable := range callables {
		counts[callable.Selector]++
	}
	labels := make(map[string]string, len(callables))
	for _, callable := range callables {
		if counts[callable.Selector] == 1 {
			labels[callable.ID] = callable.Selector
		} else {
			labels[callable.ID] = callable.ID
		}
	}
	return labels
}
