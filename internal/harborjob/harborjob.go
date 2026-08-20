// Package harborjob maps a complete local Harbor job directory into one
// canonical RLViz source without modifying or following links from the job.
package harborjob

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/atif"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

const (
	Format              = "harbor-job-directory"
	formatVersion       = "v1"
	maxJSONBytes  int64 = 256 << 20
	maxTrials           = 10_000
	maxFiles            = 100_000
)

type trialSnapshot struct {
	name         string
	directory    string
	result       string
	config       string
	lock         string
	trajectories []string
	ctrf         []string
	artifacts    []string
}

// Snapshot is a bounded, deterministic inventory of the files RLViz reads.
type Snapshot struct {
	Root        string
	Size        int64
	ModTime     time.Time
	Fingerprint string
	jobConfig   string
	jobResult   string
	trials      []trialSnapshot
}

// Inspect recognizes a complete Harbor job and inventories only regular files
// inside it. Symlinks are never followed after the root has been resolved.
func Inspect(root string) (Snapshot, error) {
	var snapshot Snapshot
	root, err := filepath.Abs(root)
	if err != nil {
		return snapshot, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return snapshot, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return snapshot, err
	}
	if !info.IsDir() {
		return snapshot, fmt.Errorf("harbor job source %q is not a directory", root)
	}
	snapshot.Root = root
	snapshot.jobConfig = filepath.Join(root, "config.json")
	snapshot.jobResult = filepath.Join(root, "result.json")
	files := make([]string, 0)
	for _, required := range []string{snapshot.jobConfig, snapshot.jobResult} {
		if err := addRegular(root, required, &files); err != nil {
			return Snapshot{}, fmt.Errorf("recognize Harbor job: %w", err)
		}
	}

	containers := []string{root}
	legacy := filepath.Join(root, "trials")
	if legacyInfo, statErr := os.Lstat(legacy); statErr == nil {
		if legacyInfo.Mode()&os.ModeSymlink != 0 || !legacyInfo.IsDir() {
			return Snapshot{}, fmt.Errorf("recognize Harbor job: trials must be a real directory")
		}
		containers = append(containers, legacy)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Snapshot{}, fmt.Errorf("recognize Harbor job: inspect trials: %w", statErr)
	}
	for _, container := range containers {
		entries, readErr := boundedReadDir(container)
		if readErr != nil {
			return Snapshot{}, readErr
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || (container == root && entry.Name() == "trials") {
				continue
			}
			directory := filepath.Join(container, entry.Name())
			result := filepath.Join(directory, "result.json")
			if _, statErr := os.Lstat(result); errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			trial := trialSnapshot{name: entry.Name(), directory: directory, result: result, config: filepath.Join(directory, "config.json"), lock: filepath.Join(directory, "lock.json")}
			if err := addRegular(root, trial.result, &files); err != nil {
				return Snapshot{}, err
			}
			for _, optional := range []*string{&trial.config, &trial.lock} {
				exists, optionalErr := addOptionalRegular(root, *optional, &files)
				if optionalErr != nil {
					return Snapshot{}, optionalErr
				}
				if !exists {
					*optional = ""
				}
			}
			if err := collectExisting(root, &files, &trial.trajectories, filepath.Join(directory, "agent", "trajectory.json")); err != nil {
				return Snapshot{}, err
			}
			if err := collectExisting(root, &files, &trial.ctrf, filepath.Join(directory, "verifier", "ctrf.json")); err != nil {
				return Snapshot{}, err
			}
			if err := collectExisting(root, &files, &trial.artifacts,
				filepath.Join(directory, "verifier", "reward.txt"), filepath.Join(directory, "verifier", "reward.json"),
				filepath.Join(directory, "verifier", "test-stdout.txt"), filepath.Join(directory, "verifier", "test-stderr.txt")); err != nil {
				return Snapshot{}, err
			}
			steps := filepath.Join(directory, "steps")
			stepEntries, stepErr := realDirectoryEntries(steps)
			if stepErr != nil && !errors.Is(stepErr, os.ErrNotExist) {
				return Snapshot{}, stepErr
			}
			if stepErr == nil {
				if len(stepEntries) > maxTrials {
					return Snapshot{}, fmt.Errorf("harbor trial %q has more than %d steps", trial.name, maxTrials)
				}
				for _, step := range stepEntries {
					if !step.IsDir() || step.Type()&os.ModeSymlink != 0 {
						continue
					}
					base := filepath.Join(steps, step.Name())
					if err := collectExisting(root, &files, &trial.trajectories, filepath.Join(base, "agent", "trajectory.json")); err != nil {
						return Snapshot{}, err
					}
					if err := collectExisting(root, &files, &trial.ctrf, filepath.Join(base, "verifier", "ctrf.json")); err != nil {
						return Snapshot{}, err
					}
					if err := collectExisting(root, &files, &trial.artifacts,
						filepath.Join(base, "verifier", "reward.txt"), filepath.Join(base, "verifier", "reward.json"),
						filepath.Join(base, "verifier", "test-stdout.txt"), filepath.Join(base, "verifier", "test-stderr.txt")); err != nil {
						return Snapshot{}, err
					}
				}
			}
			artifactRoot := filepath.Join(directory, "artifacts")
			artifactFiles, walkErr := boundedFiles(artifactRoot)
			if walkErr != nil {
				return Snapshot{}, walkErr
			}
			for _, path := range artifactFiles {
				if err := addRegular(root, path, &files); err != nil {
					return Snapshot{}, err
				}
				trial.artifacts = append(trial.artifacts, path)
			}
			snapshot.trials = append(snapshot.trials, trial)
			if len(snapshot.trials) > maxTrials {
				return Snapshot{}, fmt.Errorf("harbor job has more than %d trials", maxTrials)
			}
		}
	}
	if len(snapshot.trials) == 0 {
		return Snapshot{}, errors.New("harbor job has no trial directories containing result.json")
	}
	sort.Slice(snapshot.trials, func(i, j int) bool { return snapshot.trials[i].directory < snapshot.trials[j].directory })
	sort.Strings(files)
	hash := sha256.New()
	_, _ = io.WriteString(hash, Format+":"+formatVersion+"\n")
	for _, path := range files {
		fileInfo, statErr := os.Lstat(path)
		if statErr != nil {
			return Snapshot{}, statErr
		}
		relative, _ := filepath.Rel(root, path)
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\n", filepath.ToSlash(relative), fileInfo.Size(), fileInfo.ModTime().UnixNano())
		if fileInfo.Size() > 0 && snapshot.Size > (1<<63-1)-fileInfo.Size() {
			return Snapshot{}, errors.New("harbor job file sizes exceed supported range")
		}
		snapshot.Size += fileInfo.Size()
		if fileInfo.ModTime().After(snapshot.ModTime) {
			snapshot.ModTime = fileInfo.ModTime()
		}
	}
	snapshot.Fingerprint = Format + ":" + formatVersion + ":" + hex.EncodeToString(hash.Sum(nil))
	return snapshot, nil
}

func realDirectoryEntries(path string) ([]os.DirEntry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%q must be a real directory", path)
	}
	return boundedReadDir(path)
}

func boundedReadDir(path string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	if len(entries) > maxFiles {
		return nil, fmt.Errorf("directory %q has more than %d entries", path, maxFiles)
	}
	return entries, nil
}

func boundedFiles(root string) ([]string, error) {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("artifact root %q must be a real directory", root)
	}
	files := make([]string, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			files = append(files, path)
			if len(files) > maxFiles {
				return fmt.Errorf("artifact tree has more than %d files", maxFiles)
			}
		}
		return nil
	})
	return files, err
}

func addRegular(root, path string, files *[]string) error {
	if !inside(root, path) {
		return fmt.Errorf("source path %q escapes Harbor job root", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", relativePath(root, path), err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file, not a symlink or directory", relativePath(root, path))
	}
	if len(*files) >= maxFiles {
		return fmt.Errorf("harbor job has more than %d recognized files", maxFiles)
	}
	*files = append(*files, path)
	return nil
}

func addOptionalRegular(root, path string, files *[]string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := addRegular(root, path, files); err != nil {
		return false, err
	}
	return true, nil
}

func collectExisting(root string, files, result *[]string, paths ...string) error {
	for _, path := range paths {
		exists, err := addOptionalRegular(root, path, files)
		if err != nil {
			return err
		}
		if exists {
			*result = append(*result, path)
		}
	}
	return nil
}

func inside(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Normalize converts an inspected Harbor job into deterministic canonical
// NDJSON. It reuses the public ATIF mapping for trajectory content.
func Normalize(snapshot Snapshot) ([]byte, error) {
	jobConfig, _, err := readJSONObject(snapshot.Root, snapshot.jobConfig)
	if err != nil {
		return nil, err
	}
	jobResult, _, err := readJSONObject(snapshot.Root, snapshot.jobResult)
	if err != nil {
		return nil, err
	}
	jobID := firstString(jobResult["id"], jobConfig["job_name"], filepath.Base(snapshot.Root))
	runID := stableID("harbor-job", jobID)
	jobResultSummary := selectKeys(jobResult, "id", "started_at", "updated_at", "finished_at", "n_total_trials", "stats")
	emitter := newEmitter()
	if err := emitter.emit(&model.Run{RecordType: model.RecordRun, ID: runID, Name: firstString(jobConfig["job_name"], filepath.Base(snapshot.Root)), StartedAt: stringValue(jobResult["started_at"]), Metadata: compact(model.Metadata{"source_format": Format, "harbor_job_id": jobResult["id"], "result": jobResultSummary, "provenance": model.Metadata{"config": "config.json", "result": "result.json"}})}); err != nil {
		return nil, err
	}

	seenCases, seenGroups := map[string]bool{}, map[string]bool{}
	for trialIndex, trial := range snapshot.trials {
		result, resultRaw, readErr := readJSONObject(snapshot.Root, trial.result)
		if readErr != nil {
			return nil, fmt.Errorf("trial %s: %w", trial.name, readErr)
		}
		config := map[string]any{}
		if trial.config != "" {
			config, _, readErr = readJSONObject(snapshot.Root, trial.config)
			if readErr != nil {
				return nil, fmt.Errorf("trial %s: %w", trial.name, readErr)
			}
		}
		taskName := firstString(result["task_name"], nested(config, "task", "name"), trial.name)
		caseIdentity := firstString(result["task_id"], result["task_checksum"], taskName)
		caseID := stableID("harbor-task", runID, result["source"], caseIdentity)
		agentName := firstString(nested(result, "agent_info", "name"), nested(config, "agent", "name"), "unknown agent")
		modelName := firstString(nested(result, "agent_info", "model_info", "name"), nested(config, "agent", "model_name"))
		groupID := stableID("harbor-variant", caseID, agentName, modelName)
		documents, docErr := normalizeTrajectories(snapshot.Root, trial.trajectories)
		if docErr != nil {
			return nil, fmt.Errorf("trial %s: %w", trial.name, docErr)
		}
		if !seenCases[caseID] {
			input := any(nil)
			if len(documents) > 0 {
				input = documents[0].input
			}
			if err := emitter.emit(&model.Case{RecordType: model.RecordCase, ID: caseID, RunID: runID, Name: taskName, Input: input, Metadata: compact(model.Metadata{"source": result["source"], "task_id": result["task_id"], "task_checksum": result["task_checksum"]})}); err != nil {
				return nil, err
			}
			seenCases[caseID] = true
		}
		if !seenGroups[groupID] {
			if err := emitter.emit(&model.Group{RecordType: model.RecordGroup, ID: groupID, CaseID: caseID, Name: strings.TrimSpace(agentName + " " + modelName), Metadata: compact(model.Metadata{"agent": agentName, "model": modelName})}); err != nil {
				return nil, err
			}
			seenGroups[groupID] = true
		}
		trialID := stableID("harbor-trial", runID, relativePath(snapshot.Root, trial.directory), firstString(result["id"], result["trial_name"], trial.name))
		status, termination := trialStatus(result)
		trialSummary := selectKeys(result, "started_at", "updated_at", "finished_at", "environment_setup", "agent_setup", "agent_execution", "verifier", "exception_info")
		trialMetadata := compact(model.Metadata{"source_format": Format, "trial_name": firstString(result["trial_name"], trial.name), "trial_id": result["id"], "trial_uri": result["trial_uri"], "agent": result["agent_info"], "model": modelName, "result": trialSummary, "provenance": model.Metadata{"result": relativePath(snapshot.Root, trial.result), "config": relativePath(snapshot.Root, trial.config), "lock": relativePath(snapshot.Root, trial.lock)}})
		rootTrajectoryID := trialID
		if len(documents) != 1 {
			if err := emitter.emit(&model.Trajectory{RecordType: model.RecordTrajectory, ID: trialID, GroupID: groupID, Status: status, Termination: termination, Metadata: trialMetadata}); err != nil {
				return nil, err
			}
		}
		maxSequence := int64(-1)
		for _, document := range documents {
			rootID, maximum, emitErr := emitATIFDocument(emitter, document.records, groupID, trialID, document.key, len(documents) == 1, status, termination, trialMetadata)
			if emitErr != nil {
				return nil, emitErr
			}
			if len(documents) == 1 {
				rootTrajectoryID = rootID
			}
			if maximum > maxSequence {
				maxSequence = maximum
			}
		}
		if len(documents) == 0 {
			maxSequence = -1
		}
		nextSequence := maxSequence + 1
		for _, entry := range trialExceptions(result) {
			exception := entry.info
			eventID := stableID("harbor-exception", rootTrajectoryID, entry.step, exception["occurred_at"], exception["exception_type"])
			event := &model.Event{RecordType: model.RecordEvent, ID: eventID, TrajectoryID: rootTrajectoryID, Sequence: nextSequence, Kind: "error", Timestamp: stringValue(exception["occurred_at"]), Output: compact(exception), Source: &model.SourceLocation{Path: relativePath(snapshot.Root, trial.result)}, Raw: resultRaw, Metadata: compact(model.Metadata{"title": firstString(exception["exception_type"], "Harbor trial exception"), "step_name": entry.step, "provenance": "source_native"})}
			if err := emitter.emit(event); err != nil {
				return nil, err
			}
			nextSequence++
		}
		for _, ctrfPath := range trial.ctrf {
			ctrf, raw, ctrfErr := readJSON(snapshot.Root, ctrfPath)
			if ctrfErr != nil {
				return nil, ctrfErr
			}
			verdict, reason := ctrfOutcome(ctrf)
			eventID := stableID("harbor-ctrf", rootTrajectoryID, relativePath(snapshot.Root, ctrfPath))
			output := compact(model.Metadata{"verdict": verdict, "reason": reason})
			stepName := trialStepName(trial.directory, ctrfPath)
			alignmentKey := "grader:ctrf"
			if stepName != "" {
				alignmentKey += ":" + stepName
			}
			event := &model.Event{RecordType: model.RecordEvent, ID: eventID, TrajectoryID: rootTrajectoryID, Sequence: nextSequence, Kind: "grader", AlignmentKey: alignmentKey, Output: output, Data: ctrf, Source: &model.SourceLocation{Path: relativePath(snapshot.Root, ctrfPath)}, Raw: raw, Metadata: compact(model.Metadata{"title": "CTRF verifier", "step_name": stepName, "provenance": "source_native"})}
			if err := emitter.emit(event); err != nil {
				return nil, err
			}
			nextSequence++
		}
		if err := emitSignals(emitter, rootTrajectoryID, result); err != nil {
			return nil, err
		}
		artifactPaths := append([]string{}, trial.artifacts...)
		artifactPaths = append(artifactPaths, trial.ctrf...)
		artifactPaths = append(artifactPaths, trial.config, trial.lock, trial.result)
		if trialIndex == 0 {
			artifactPaths = append(artifactPaths, snapshot.jobConfig, snapshot.jobResult)
		}
		if err := emitArtifacts(emitter, snapshot.Root, rootTrajectoryID, artifactPaths); err != nil {
			return nil, err
		}
	}
	if err := emitter.emit(&model.Complete{RecordType: model.RecordComplete, Records: emitter.records, Warnings: 0}); err != nil {
		return nil, err
	}
	return emitter.buffer.Bytes(), nil
}

func trialStepName(trialDirectory, path string) string {
	relative, err := filepath.Rel(trialDirectory, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) >= 3 && parts[0] == "steps" {
		return parts[1]
	}
	return ""
}

type normalizedDocument struct {
	records []*model.Record
	input   any
	key     string
}

func normalizeTrajectories(root string, paths []string) ([]normalizedDocument, error) {
	documents := make([]normalizedDocument, 0, len(paths))
	for _, path := range paths {
		raw, err := readBounded(path)
		if err != nil {
			return nil, err
		}
		canonical, err := atif.NormalizeBytes(raw, relativePath(root, path))
		if err != nil {
			return nil, fmt.Errorf("normalize %s: %w", relativePath(root, path), err)
		}
		decoder := model.NewDecoder(bytes.NewReader(canonical))
		document := normalizedDocument{key: relativePath(root, path)}
		for {
			record, decodeErr := decoder.Next()
			if errors.Is(decodeErr, io.EOF) {
				break
			}
			if decodeErr != nil {
				return nil, decodeErr
			}
			if value, ok := record.Value.(*model.Case); ok {
				document.input = value.Input
			}
			document.records = append(document.records, record)
		}
		documents = append(documents, document)
	}
	return documents, nil
}

func emitATIFDocument(emitter *recordEmitter, records []*model.Record, groupID, trialID, documentKey string, single bool, status, termination string, metadata model.Metadata) (string, int64, error) {
	trajectoryIDs := map[string]string{}
	eventIDs := map[string]string{}
	branchIDs := map[string]string{}
	rootOldID, rootNewID := "", trialID
	for _, record := range records {
		switch value := record.Value.(type) {
		case *model.Trajectory:
			trajectory := value
			if rootOldID == "" {
				rootOldID = trajectory.ID
			}
			trajectoryIDs[trajectory.ID] = stableID("harbor-atif-trajectory", trialID, documentKey, trajectory.ID)
			if trajectory.BranchID != "" {
				branchIDs[trajectory.BranchID] = stableID("harbor-atif-branch", trialID, documentKey, trajectory.BranchID)
			}
		case *model.Event:
			eventIDs[value.ID] = stableID("harbor-atif-event", trialID, documentKey, value.ID)
			if value.BranchID != "" {
				branchIDs[value.BranchID] = stableID("harbor-atif-branch", trialID, documentKey, value.BranchID)
			}
		}
	}
	if single && rootOldID != "" {
		trajectoryIDs[rootOldID] = trialID
	}
	maximum := int64(-1)
	for _, record := range records {
		switch value := record.Value.(type) {
		case *model.Run, *model.Case, *model.Group, *model.Complete:
			continue
		case *model.Trajectory:
			oldID := value.ID
			value.ID = trajectoryIDs[oldID]
			value.GroupID = groupID
			value.BranchID = branchIDs[value.BranchID]
			if oldID == rootOldID {
				rootNewID = value.ID
				if single {
					value.Status, value.Termination = status, termination
					value.Metadata = merge(value.Metadata, metadata)
				} else {
					value.ParentID = trialID
				}
			} else if mapped := trajectoryIDs[value.ParentID]; mapped != "" {
				value.ParentID = mapped
			}
		case *model.Event:
			value.ID = eventIDs[value.ID]
			value.TrajectoryID = trajectoryIDs[value.TrajectoryID]
			value.ParentID = eventIDs[value.ParentID]
			value.BranchID = branchIDs[value.BranchID]
			if value.Context != nil {
				value.Context.RetainedEventIDs = remapIDs(value.Context.RetainedEventIDs, eventIDs)
				value.Context.DroppedEventIDs = remapIDs(value.Context.DroppedEventIDs, eventIDs)
				value.Context.SummarizedEventIDs = remapIDs(value.Context.SummarizedEventIDs, eventIDs)
			}
			if value.TrajectoryID == rootNewID && value.Sequence > maximum {
				maximum = value.Sequence
			}
		case *model.Signal:
			value.ID = stableID("harbor-atif-signal", trialID, documentKey, value.ID)
			value.TrajectoryID = trajectoryIDs[value.TrajectoryID]
			value.EventID = eventIDs[value.EventID]
		case *model.Artifact:
			value.ID = stableID("harbor-atif-artifact", trialID, documentKey, value.ID)
			value.TrajectoryID = trajectoryIDs[value.TrajectoryID]
			value.EventID = eventIDs[value.EventID]
		}
		if err := emitter.emit(record.Value); err != nil {
			return "", maximum, err
		}
	}
	return rootNewID, maximum, nil
}

func remapIDs(values []string, ids map[string]string) []string {
	for index, value := range values {
		if mapped := ids[value]; mapped != "" {
			values[index] = mapped
		}
	}
	return values
}

func emitSignals(emitter *recordEmitter, trajectoryID string, result map[string]any) error {
	values := map[string]any{}
	provenance := map[string]string{}
	for name, value := range object(nested(result, "verifier_result", "rewards")) {
		values[name] = value
		provenance[name] = "source_native"
	}
	contexts := []map[string]any{}
	if context := object(result["agent_result"]); context != nil {
		contexts = append(contexts, context)
	}
	hasAggregateContext := len(contexts) > 0
	for stepIndex, raw := range array(result["step_results"]) {
		step := object(raw)
		if !hasAggregateContext {
			if context := object(step["agent_result"]); context != nil {
				contexts = append(contexts, context)
			}
		}
		stepName := firstString(step["step_name"], step["id"], strconv.Itoa(stepIndex+1))
		for name, value := range object(nested(step, "verifier_result", "rewards")) {
			signalName := "step." + stepName + "." + name
			values[signalName] = value
			provenance[signalName] = "source_native"
		}
	}
	for _, field := range []string{"n_input_tokens", "n_cache_tokens", "n_output_tokens", "cost_usd"} {
		var total float64
		found, integerOnly := false, true
		for _, context := range contexts {
			if number, ok := number(context[field]); ok {
				total += number
				found = true
				integerOnly = integerOnly && number == float64(int64(number))
			}
		}
		if found {
			name := map[string]string{"n_input_tokens": "input_tokens", "n_cache_tokens": "cached_input_tokens", "n_output_tokens": "output_tokens", "cost_usd": "cost_usd"}[field]
			if integerOnly {
				values[name] = int64(total)
			} else {
				values[name] = total
			}
			provenance[name] = "adapter_derived_sum"
		}
	}
	inputTokens, hasInput := number(values["input_tokens"])
	cacheTokens, hasCache := number(values["cached_input_tokens"])
	outputTokens, hasOutput := number(values["output_tokens"])
	if hasInput && hasOutput {
		values["total_tokens"] = numericTotal(inputTokens + outputTokens)
		provenance["total_tokens"] = "adapter_derived_sum"
	}
	if hasInput && hasCache && cacheTokens <= inputTokens {
		values["uncached_input_tokens"] = numericTotal(inputTokens - cacheTokens)
		provenance["uncached_input_tokens"] = "adapter_derived_difference"
		if inputTokens > 0 {
			values["cached_input_rate"] = cacheTokens / inputTokens
			provenance["cached_input_rate"] = "adapter_derived_ratio"
		}
	}
	if duration, ok := durationMillis(result["started_at"], result["finished_at"]); ok {
		values["duration_ms"] = duration
		provenance["duration_ms"] = "adapter_derived_difference"
	}
	for _, phase := range []string{"environment_setup", "agent_setup", "agent_execution", "verifier"} {
		timing := object(result[phase])
		if duration, ok := durationMillis(timing["started_at"], timing["finished_at"]); ok {
			name := phase + "_duration_ms"
			values[name] = duration
			provenance[name] = "adapter_derived_difference"
		}
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := emitter.emit(&model.Signal{RecordType: model.RecordSignal, ID: stableID("harbor-signal", trajectoryID, name), TrajectoryID: trajectoryID, Name: name, Value: values[name], Metadata: model.Metadata{"provenance": provenance[name]}}); err != nil {
			return err
		}
	}
	return nil
}

func emitArtifacts(emitter *recordEmitter, root, trajectoryID string, paths []string) error {
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		relative := relativePath(root, path)
		mediaType := mime.TypeByExtension(filepath.Ext(path))
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		artifact := &model.Artifact{RecordType: model.RecordArtifact, ID: stableID("harbor-artifact", trajectoryID, relative), TrajectoryID: trajectoryID, Name: filepath.Base(path), MediaType: mediaType, Path: relative, Metadata: model.Metadata{"provenance": "source_native", "source_document": relative}}
		if err := emitter.emit(artifact); err != nil {
			return err
		}
	}
	return nil
}

func ctrfOutcome(value any) (string, string) {
	summary := object(nested(object(value), "results", "summary"))
	failed, failedOK := number(summary["failed"])
	tests, testsOK := number(summary["tests"])
	if !failedOK || !testsOK || tests <= 0 {
		return "", ""
	}
	verdict := "pass"
	if failed > 0 {
		verdict = "fail"
	}
	return verdict, fmt.Sprintf("CTRF reported %s failed of %s tests.", formatNumber(failed), formatNumber(tests))
}

func trialStatus(result map[string]any) (string, string) {
	if exceptions := trialExceptions(result); len(exceptions) > 0 {
		return "failed", stringValue(exceptions[0].info["exception_type"])
	}
	if stringValue(result["finished_at"]) != "" {
		return "completed", ""
	}
	return "in_progress", ""
}

type trialException struct {
	step string
	info map[string]any
}

func trialExceptions(result map[string]any) []trialException {
	exceptions := make([]trialException, 0)
	if exception := object(result["exception_info"]); exception != nil {
		exceptions = append(exceptions, trialException{info: exception})
	}
	for index, raw := range array(result["step_results"]) {
		step := object(raw)
		if exception := object(step["exception_info"]); exception != nil {
			exceptions = append(exceptions, trialException{step: firstString(step["step_name"], step["id"], strconv.Itoa(index+1)), info: exception})
		}
	}
	return exceptions
}

type recordEmitter struct {
	buffer  bytes.Buffer
	records int64
}

func newEmitter() *recordEmitter { return &recordEmitter{} }

func (emitter *recordEmitter) emit(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	emitter.buffer.Write(raw)
	emitter.buffer.WriteByte('\n')
	if _, ok := value.(*model.Complete); !ok {
		emitter.records++
	}
	return nil
}

func readJSONObject(root, path string) (map[string]any, json.RawMessage, error) {
	value, raw, err := readJSON(root, path)
	if err != nil {
		return nil, nil, err
	}
	object := object(value)
	if object == nil {
		return nil, nil, fmt.Errorf("%s must contain a JSON object", relativePath(root, path))
	}
	return object, raw, nil
}

func readJSON(root, path string) (any, json.RawMessage, error) {
	raw, err := readBounded(path)
	if err != nil {
		return nil, nil, err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, nil, fmt.Errorf("decode %s: %w", relativePath(root, path), err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, nil, fmt.Errorf("decode %s: multiple JSON values", relativePath(root, path))
		}
		return nil, nil, fmt.Errorf("decode %s: %w", relativePath(root, path), err)
	}
	return value, raw, nil
}

func readBounded(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source file %q is not regular", path)
	}
	if info.Size() > maxJSONBytes {
		return nil, fmt.Errorf("source file %q exceeds %d bytes", path, maxJSONBytes)
	}
	return os.ReadFile(path)
}

func relativePath(root, path string) string {
	if path == "" {
		return ""
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func stableID(prefix string, values ...any) string {
	hash := sha256.New()
	for _, value := range values {
		_, _ = fmt.Fprintf(hash, "%v\x00", value)
	}
	return prefix + "-" + hex.EncodeToString(hash.Sum(nil)[:10])
}

func object(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func array(value any) []any {
	result, _ := value.([]any)
	return result
}

func nested(value any, keys ...string) any {
	current := value
	for _, key := range keys {
		current = object(current)[key]
	}
	return current
}

func stringValue(value any) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func firstString(values ...any) string {
	for _, value := range values {
		if result := stringValue(value); result != "" {
			return result
		}
	}
	return ""
}

func number(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		result, err := value.Float64()
		return result, err == nil
	case float64:
		return value, true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	default:
		return 0, false
	}
}

func formatNumber(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }

func numericTotal(value float64) any {
	if value == float64(int64(value)) {
		return int64(value)
	}
	return value
}

func durationMillis(startValue, finishValue any) (float64, bool) {
	start, startErr := time.Parse(time.RFC3339Nano, stringValue(startValue))
	finish, finishErr := time.Parse(time.RFC3339Nano, stringValue(finishValue))
	if startErr != nil || finishErr != nil || finish.Before(start) {
		return 0, false
	}
	return float64(finish.Sub(start)) / float64(time.Millisecond), true
}

func selectKeys(source map[string]any, keys ...string) map[string]any {
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := source[key]; ok && value != nil {
			result[key] = value
		}
	}
	return result
}

func merge(left, right model.Metadata) model.Metadata {
	result := model.Metadata{}
	for key, value := range left {
		result[key] = value
	}
	for key, value := range right {
		result[key] = value
	}
	return result
}

func compact(metadata model.Metadata) model.Metadata {
	for key, value := range metadata {
		if value == nil {
			delete(metadata, key)
			continue
		}
		if text, ok := value.(string); ok && text == "" {
			delete(metadata, key)
		}
	}
	return metadata
}
