package evidence

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

type sarifLog struct {
	Version string `json:"version"`
	Runs    *[]struct {
		Tool struct {
			Driver struct {
				Name string `json:"name"`
			} `json:"driver"`
		} `json:"tool"`
		OriginalURIBaseIDs map[string]sarifArtifactLocation `json:"originalUriBaseIds"`
		// Artifacts are the files the tool analysed.
		Artifacts []struct {
			Location sarifArtifactLocation `json:"location"`
		} `json:"artifacts"`
		Results []struct {
			RuleID    string `json:"ruleId"`
			Level     string `json:"level"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
				} `json:"physicalLocation"`
			} `json:"locations"`
		} `json:"results"`
	} `json:"runs"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type ruleCount struct {
	Rule  string `json:"rule"`
	Level string `json:"level"`
	Count int    `json:"count"`
}

const maxTopRules = 10

func parseSARIF(data []byte) (specv1.EvidenceOutcome, any, []string, error) {
	var log sarifLog
	if err := json.Unmarshal(data, &log); err != nil {
		return "", nil, nil, fmt.Errorf("%w: not a SARIF log: %w", ErrUnsupported, err)
	}
	if log.Runs == nil || !strings.HasPrefix(log.Version, "2.") {
		return "", nil, nil, fmt.Errorf("%w: not a SARIF 2.x log (version %q, runs present: %t)", ErrUnsupported, log.Version, log.Runs != nil)
	}

	counts := map[string]int{"error": 0, "warning": 0, "note": 0}
	rules := map[string]*ruleCount{}
	var tools []string
	seenTool := map[string]bool{}
	total := 0
	for _, run := range *log.Runs {
		if name := run.Tool.Driver.Name; name != "" && !seenTool[name] {
			seenTool[name] = true
			tools = append(tools, name)
		}
		for _, r := range run.Results {
			total++
			// SARIF: a result with no level is a "warning"; level "none" is
			// informational. Anything unrecognised is treated as a warning so
			// it is never silently dropped.
			level := strings.ToLower(r.Level)
			switch level {
			case "error", "warning", "note":
			case "none":
				level = "note"
			default:
				level = "warning"
			}
			counts[level]++
			id := r.RuleID
			if id == "" {
				id = "(no rule id)"
			}
			rc := rules[id+"|"+level]
			if rc == nil {
				rc = &ruleCount{Rule: id, Level: level}
				rules[id+"|"+level] = rc
			}
			rc.Count++
		}
	}

	top := make([]ruleCount, 0, len(rules))
	for _, rc := range rules {
		top = append(top, *rc)
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Count != top[j].Count {
			return top[i].Count > top[j].Count
		}
		if top[i].Rule != top[j].Rule {
			return top[i].Rule < top[j].Rule
		}
		return top[i].Level < top[j].Level
	})
	if len(top) > maxTopRules {
		top = top[:maxTopRules]
	}

	outcome := specv1.EvidenceOutcomePass
	switch {
	case counts["error"] > 0:
		outcome = specv1.EvidenceOutcomeFail
	case len(*log.Runs) == 0:
		outcome = specv1.EvidenceOutcomeInconclusive // no tool ran
	}
	if tools == nil {
		tools = []string{}
	}
	payload := map[string]any{"format": "sarif", "tools": tools, "results": total, "counts": counts, "top_rules": top}
	return outcome, payload, sarifFiles(log), nil
}

// sarifFiles lists the files a SARIF log covers: every run's analysed files
// (run.artifacts) plus the files its results point at. The files with results
// alone are not enough: a clean scan has none, yet it depends on every file it
// read. So unless every run lists its artifacts, the log gives no footprint.
func sarifFiles(log sarifLog) []string {
	if log.Runs == nil || len(*log.Runs) == 0 {
		return nil
	}
	var files []string
	for _, run := range *log.Runs {
		if len(run.Artifacts) == 0 {
			return nil
		}
		locations := make([]sarifArtifactLocation, 0, len(run.Artifacts))
		for _, a := range run.Artifacts {
			locations = append(locations, a.Location)
		}
		for _, r := range run.Results {
			for _, l := range r.Locations {
				locations = append(locations, l.PhysicalLocation.ArtifactLocation)
			}
		}
		for _, loc := range locations {
			p, ok := sarifPath(loc, run.OriginalURIBaseIDs)
			if !ok {
				return nil
			}
			files = append(files, p)
		}
	}
	return files
}

// sarifPath turns an artifact location into a file path. A relative URI with a
// uriBaseId defined in originalUriBaseIds is joined onto that base; an
// undefined base (commonly %SRCROOT%) is taken to be the repository root.
func sarifPath(loc sarifArtifactLocation, bases map[string]sarifArtifactLocation) (string, bool) {
	p, ok := uriPath(loc.URI)
	if !ok {
		return "", false
	}
	if base, defined := bases[loc.URIBaseID]; loc.URIBaseID != "" && defined && !path.IsAbs(p) {
		if base.URIBaseID != "" {
			return "", false // nested bases are not followed
		}
		b, ok := uriPath(base.URI)
		if !ok {
			return "", false
		}
		p = path.Join(b, p)
	}
	return p, true
}

// uriPath reads a relative or file:// URI as a path.
func uriPath(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || uri == "" {
		return "", false
	}
	switch {
	case u.Scheme == "":
		return u.Path, u.Path != ""
	case u.Scheme == "file" && (u.Host == "" || u.Host == "localhost"):
		return u.Path, u.Path != ""
	}
	return "", false
}
