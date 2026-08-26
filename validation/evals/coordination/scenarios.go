package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/hosted"
)

type scenarioDefinition struct {
	ID              string
	Name            string
	IntentA         intentScript
	IntentB         intentScript
	ExpectedRouting string
}

type intentScript struct {
	Title            string
	Outcome          string
	Approach         string
	Contracts        []string
	AnticipatedPaths []string
}

var scenarioDefinitions = []scenarioDefinition{
	{
		ID: "A", Name: "contract change under a consumer", ExpectedRouting: "next-turn",
		IntentA: intentScript{Title: "Consume refresh contract", Outcome: "Build the frontend flow against backend.Refresh(userID).", Contracts: []string{"backend.Refresh"}, AnticipatedPaths: []string{"frontend/session.ts"}},
		IntentB: intentScript{Title: "Revise refresh contract", Outcome: "Change backend.Refresh to require a session identifier and policy.", Contracts: []string{"backend.Refresh"}, AnticipatedPaths: []string{"backend/refresh.go"}},
	},
	{
		ID: "B", Name: "semantic duplication", ExpectedRouting: "next-turn",
		IntentA: intentScript{Title: "Revoke credentials", Outcome: "Rotate browser sessions after privilege changes and revoke prior credentials.", AnticipatedPaths: []string{"backend/revoke.go"}},
		IntentB: intentScript{Title: "Clean up sessions", Outcome: "Issue new web login credentials after a member role changes and invalidate old credentials.", AnticipatedPaths: []string{"frontend/session_cleanup.ts"}},
	},
	{
		ID: "C", Name: "interface changed after session start", ExpectedRouting: "next-turn",
		IntentA: intentScript{Title: "Use refresh interface", Outcome: "Complete the frontend refresh flow using the interface observed at session start.", Contracts: []string{"backend.Refresh"}, AnticipatedPaths: []string{"frontend/session.ts"}},
		IntentB: intentScript{Title: "Change refresh interface", Outcome: "Replace the refresh user identifier with a session identifier and policy.", Contracts: []string{"backend.Refresh"}, AnticipatedPaths: []string{"backend/refresh.go"}},
	},
	{
		ID: "D", Name: "dependency wait", ExpectedRouting: "next-turn",
		IntentA: intentScript{Title: "Wait for session API", Outcome: "Complete the frontend only after the session API contract exists.", Approach: "waiting_on session-api contract", Contracts: []string{"session-api"}, AnticipatedPaths: []string{"frontend/session.ts"}},
		IntentB: intentScript{Title: "Create session API", Outcome: "Publish and verify the exported session API contract.", Contracts: []string{"session-api"}, AnticipatedPaths: []string{"backend/session_api.go"}},
	},
	{
		ID: "E", Name: "true independence", ExpectedRouting: "silence",
		IntentA: intentScript{Title: "Adjust theme color", Outcome: "Change the frontend accent color without touching backend behavior.", AnticipatedPaths: []string{"frontend/theme.ts"}},
		IntentB: intentScript{Title: "Rename audit category", Outcome: "Rename a backend audit category without changing frontend presentation.", AnticipatedPaths: []string{"backend/audit.go"}},
	},
	{
		ID: "F", Name: "same file unrelated regions", ExpectedRouting: "next-turn",
		IntentA: intentScript{Title: "Rename navigation label", Outcome: "Change only the navigation label in shared settings.", AnticipatedPaths: []string{"shared/settings.ts"}},
		IntentB: intentScript{Title: "Increase retry limit", Outcome: "Change only the retry limit in shared settings.", AnticipatedPaths: []string{"shared/settings.ts"}},
	},
	{
		ID: "G", Name: "WIP uncertainty", ExpectedRouting: "next-turn",
		IntentA: intentScript{Title: "Consume refresh contract", Outcome: "Continue frontend work from the observed refresh contract.", Contracts: []string{"backend.Refresh"}, AnticipatedPaths: []string{"frontend/session.ts"}},
		IntentB: intentScript{Title: "Prototype refresh contract", Outcome: "Publish an unverified work-in-progress refresh contract change.", Contracts: []string{"backend.Refresh"}, AnticipatedPaths: []string{"backend/refresh.go"}},
	},
}

type scenarioObservation struct {
	briefs          []hosted.CoordinationBrief
	adjustmentProbe bool
	// injectedContext is what the real turn-boundary hook delivered into the
	// reading session's next turn. Empty means the hook injected nothing.
	injectedContext string
	deliveryWait    time.Duration
}

const queueDrainTimeout = 5 * time.Second

// deliveryTimeout bounds how long a scenario keeps opening turn boundaries
// while waiting for asynchronous hosted evaluation to produce a routed item.
const deliveryTimeout = 25 * time.Second

func runScenario(definition scenarioDefinition, evaluation *evaluationEnvironment, binary string, required map[capability]bool) scenarioReport {
	started := time.Now()
	report := scenarioReport{ID: definition.ID, Name: definition.Name, ExpectedRouting: definition.ExpectedRouting}
	environment, err := evaluation.openScenario(binary, definition.ID)
	if err != nil {
		report.ExecutionError = err.Error()
		report.Metrics.WallTimeMillis = time.Since(started).Milliseconds()
		finalizeScenario(&report)
		return report
	}
	defer environment.stop()

	beginA, err := beginWork(environment, environment.agentA, environment.workspaceA, "a", definition.IntentA)
	if err == nil {
		_, err = beginWork(environment, environment.agentB, environment.workspaceB, "b", definition.IntentB)
	}
	if err == nil {
		err = environment.waitForQueue(queueDrainTimeout)
	}
	observation := scenarioObservation{}
	if beginA.Brief != nil {
		observation.briefs = append(observation.briefs, *beginA.Brief)
	}
	if err == nil {
		err = executeScenario(definition.ID, environment, beginA, &observation)
	}
	if err == nil {
		err = environment.waitForQueue(queueDrainTimeout)
	}
	if err != nil {
		report.ExecutionError = err.Error()
		report.Metrics.WallTimeMillis = time.Since(started).Milliseconds()
		finalizeScenario(&report)
		return report
	}

	findings, err := environment.findings()
	if err != nil {
		report.ExecutionError = fmt.Sprintf("read scenario findings: %v", err)
		report.Metrics.WallTimeMillis = time.Since(started).Milliseconds()
		finalizeScenario(&report)
		return report
	}
	report.ExpectedFindings = expectedFindings(definition.ID, environment)
	report.ActualFindings = annotateAdvisoryActions(findings, observation.briefs)
	report.ActualRouting = actualRouting(report.ActualFindings, observation.briefs)
	report.Assertions = scenarioAssertions(definition.ID, environment, report.ActualFindings, observation, required)
	report.Metrics = scenarioMetricValues(definition, environment, report.ExpectedFindings, report.ActualFindings, observation, time.Since(started))
	finalizeScenario(&report)

	if finishErr := finishScenario(environment); finishErr != nil {
		report.ExecutionError = finishErr.Error()
		finalizeScenario(&report)
	}
	return report
}

func beginWork(environment *scenarioEnvironment, agent *mcpAgent, workspace config.Workspace, suffix string, script intentScript) (mcpOutput, error) {
	arguments := map[string]any{
		"workspace_id": workspace.ID, "idempotency_key": "begin_" + strings.ToLower(suffix),
		"title": script.Title, "outcome": script.Outcome,
	}
	if script.Approach != "" {
		arguments["approach"] = script.Approach
	}
	if len(script.Contracts) > 0 {
		arguments["contracts"] = script.Contracts
	}
	if len(script.AnticipatedPaths) > 0 {
		arguments["anticipated_paths"] = script.AnticipatedPaths
	}
	output, err := agent.call(environment.ctx, "begin_work", arguments)
	if err != nil {
		return mcpOutput{}, err
	}
	if output.WorkspaceID != workspace.ID || output.WorkstreamID != workspace.WorkstreamID || output.IntentRevision != 1 {
		return mcpOutput{}, fmt.Errorf("begin_work returned wrong workspace/workstream/revision")
	}
	return output, nil
}

func executeScenario(id string, environment *scenarioEnvironment, beginA mcpOutput, observation *scenarioObservation) error {
	switch id {
	case "A":
		if err := environment.hookRead("backend/refresh.go"); err != nil {
			return err
		}
		// The scenario means "read first, change later". Local queues flush per
		// workspace without cross-workspace ordering, so drain before changing
		// or the read-set can arrive hosted-side after the change it must
		// invalidate.
		if err := environment.waitForQueue(queueDrainTimeout); err != nil {
			return err
		}
		if err := changeRefreshContract(environment.repository.worktreeB); err != nil {
			return err
		}
		if err := checkpointAndCheck(environment, observation, "a", "Changed backend.Refresh from userID to sessionID plus policy.", "passed"); err != nil {
			return err
		}
		if err := environment.waitForQueue(queueDrainTimeout); err != nil {
			return err
		}
		injected, waited, err := environment.hookPromptUntilDelivered("Continue the frontend refresh flow.", deliveryTimeout)
		if err != nil {
			return err
		}
		observation.injectedContext = injected
		observation.deliveryWait = waited
		return nil
	case "B":
		if err := writeFixtureFile(environment.repository.worktreeA, "backend/revoke.go", "package backend\n\nfunc RevokeCredentials(memberID string) {}\n"); err != nil {
			return err
		}
		if err := writeFixtureFile(environment.repository.worktreeB, "frontend/session_cleanup.ts", "export function cleanMemberSessions(memberID: string): void { void memberID; }\n"); err != nil {
			return err
		}
		if err := environment.forceScan(); err != nil {
			return err
		}
		if _, err := reportCheckpoint(environment, environment.agentA, environment.workspaceA, "b-a", "Implemented credential revocation behavior.", "passed", ""); err != nil {
			return err
		}
		if _, err := reportCheckpoint(environment, environment.agentB, environment.workspaceB, "b-b", "Implemented equivalent session cleanup behavior.", "passed", ""); err != nil {
			return err
		}
		return checkBoth(environment, observation)
	case "C":
		if err := environment.hookRead("backend/refresh.go"); err != nil {
			return err
		}
		// The scenario means "read first, change later". Local queues flush per
		// workspace without cross-workspace ordering, so drain before changing
		// or the read-set can arrive hosted-side after the change it must
		// invalidate.
		if err := environment.waitForQueue(queueDrainTimeout); err != nil {
			return err
		}
		if err := changeRefreshContract(environment.repository.worktreeB); err != nil {
			return err
		}
		if err := checkpointAndCheck(environment, observation, "c", "Changed the interface after the consumer session began.", "passed"); err != nil {
			return err
		}
		if err := environment.waitForQueue(queueDrainTimeout); err != nil {
			return err
		}
		injected, waited, err := environment.hookPromptUntilDelivered("Continue the frontend refresh flow.", deliveryTimeout)
		if err != nil {
			return err
		}
		observation.injectedContext = injected
		observation.deliveryWait = waited
		// The agent adjusts because the correction arrived in its turn, not
		// because a human relayed it.
		if strings.Contains(injected, "Refresh") {
			if _, err := os.ReadFile(filepath.Join(environment.repository.worktreeA, "backend", "refresh.go")); err != nil {
				return fmt.Errorf("adjustment probe re-read contract: %w", err)
			}
			output, err := environment.agentA.call(environment.ctx, "update_intent", map[string]any{
				"workspace_id": environment.workspaceA.ID, "idempotency_key": "adjust_c", "revision": int64(1),
				"title": "Adjust to refreshed interface", "outcome": "Re-read backend.Refresh and adapt the frontend to sessionID plus policy.",
				"contracts": []string{"backend.Refresh"},
			})
			if err != nil {
				return err
			}
			observation.adjustmentProbe = output.IntentRevision == 2
		}
		return nil
	case "D":
		if _, err := reportCheckpoint(environment, environment.agentB, environment.workspaceB, "d-wip", "Session API shape is drafted but remains unverified.", "not_run", ""); err != nil {
			return err
		}
		if err := writeFixtureFile(environment.repository.worktreeB, "backend/session_api.go", "package backend\n\n// SessionAPI is the exported session contract.\ntype SessionAPI struct { ID string }\n"); err != nil {
			return err
		}
		if err := environment.forceScan(); err != nil {
			return err
		}
		if _, err := reportCheckpoint(environment, environment.agentB, environment.workspaceB, "d-ready", "Exported and verified the session API contract.", "passed", ""); err != nil {
			return err
		}
		return checkAgentA(environment, observation)
	case "E":
		if err := writeFixtureFile(environment.repository.worktreeA, "frontend/theme.ts", "export const accentColor = \"green\";\n"); err != nil {
			return err
		}
		if err := writeFixtureFile(environment.repository.worktreeB, "backend/audit.go", "package backend\n\nconst AuditCategory = \"access-event\"\n"); err != nil {
			return err
		}
		if err := environment.forceScan(); err != nil {
			return err
		}
		if _, err := reportCheckpoint(environment, environment.agentA, environment.workspaceA, "e-a", "Frontend palette now renders emerald accents.", "", ""); err != nil {
			return err
		}
		if _, err := reportCheckpoint(environment, environment.agentB, environment.workspaceB, "e-b", "Backend access events now use the security-audit category.", "", ""); err != nil {
			return err
		}
		if err := checkBoth(environment, observation); err != nil {
			return err
		}
		// Silence has to hold on the delivery channel, so the turn-boundary
		// hook runs here exactly as it does in A and C.
		injected, err := environment.hookPrompt("Continue the palette work.")
		if err != nil {
			return err
		}
		observation.injectedContext = injected
		return nil
	case "F":
		if err := writeFixtureFile(environment.repository.worktreeA, "shared/settings.ts", "export const navigationLabel = \"Active sessions\";\n\nexport const retryLimit = 3;\n"); err != nil {
			return err
		}
		if err := writeFixtureFile(environment.repository.worktreeB, "shared/settings.ts", "export const navigationLabel = \"Sessions\";\n\nexport const retryLimit = 5;\n"); err != nil {
			return err
		}
		if err := environment.forceScan(); err != nil {
			return err
		}
		if _, err := reportCheckpoint(environment, environment.agentA, environment.workspaceA, "f-a", "Changed only the navigation label region.", "passed", ""); err != nil {
			return err
		}
		if _, err := reportCheckpoint(environment, environment.agentB, environment.workspaceB, "f-b", "Changed only the retry limit region.", "passed", ""); err != nil {
			return err
		}
		return checkBoth(environment, observation)
	case "G":
		if err := environment.hookRead("backend/refresh.go"); err != nil {
			return err
		}
		// The scenario means "read first, change later". Local queues flush per
		// workspace without cross-workspace ordering, so drain before changing
		// or the read-set can arrive hosted-side after the change it must
		// invalidate.
		if err := environment.waitForQueue(queueDrainTimeout); err != nil {
			return err
		}
		if err := changeRefreshContract(environment.repository.worktreeB); err != nil {
			return err
		}
		return checkpointAndCheck(environment, observation, "g", "Work-in-progress backend.Refresh signature; verification has not run.", "not_run")
	default:
		return fmt.Errorf("unknown scenario %s", id)
	}
}

func changeRefreshContract(root string) error {
	return writeFixtureFile(root, "backend/refresh.go", `package backend

type Policy struct {
	Force bool
}

func Refresh(sessionID string, policy Policy) string {
	if policy.Force {
		return "forced:" + sessionID
	}
	return "session:" + sessionID
}
`)
}

func checkpointAndCheck(environment *scenarioEnvironment, observation *scenarioObservation, suffix, summary, verificationState string) error {
	if err := environment.forceScan(); err != nil {
		return err
	}
	if _, err := reportCheckpoint(environment, environment.agentB, environment.workspaceB, suffix, summary, verificationState, ""); err != nil {
		return err
	}
	if err := environment.waitForQueue(queueDrainTimeout); err != nil {
		return err
	}
	return checkAgentA(environment, observation)
}

func reportCheckpoint(environment *scenarioEnvironment, agent *mcpAgent, workspace config.Workspace, suffix, summary, state, briefID string) (mcpOutput, error) {
	// The current local serializer adds verification[].source while the hosted
	// wire gate rejects every nested key named source. Lane A cannot change
	// either boundary, so the bounded summary carries the explicit state until
	// the owning contract lane resolves that conflict.
	if state != "" {
		summary += " Verification state: " + state + "."
	}
	arguments := map[string]any{
		"workspace_id": workspace.ID, "checkpoint_id": "chk_" + strings.ReplaceAll(suffix, "-", "_"),
		"summary": summary,
	}
	if briefID != "" {
		arguments["based_on_brief_id"] = briefID
	}
	return agent.call(environment.ctx, "report_checkpoint", arguments)
}

func checkAgentA(environment *scenarioEnvironment, observation *scenarioObservation) error {
	output, err := environment.agentA.call(environment.ctx, "check_coordination", map[string]any{
		"workspace_id": environment.workspaceA.ID, "trigger": "refresh", "approximate_token_budget": 800,
	})
	if err != nil {
		return err
	}
	if output.Brief != nil {
		observation.briefs = append(observation.briefs, *output.Brief)
	}
	return nil
}

func checkBoth(environment *scenarioEnvironment, observation *scenarioObservation) error {
	if err := environment.waitForQueue(queueDrainTimeout); err != nil {
		return err
	}
	if err := checkAgentA(environment, observation); err != nil {
		return err
	}
	output, err := environment.agentB.call(environment.ctx, "check_coordination", map[string]any{
		"workspace_id": environment.workspaceB.ID, "trigger": "refresh", "approximate_token_budget": 800,
	})
	if err != nil {
		return err
	}
	if output.Brief != nil {
		observation.briefs = append(observation.briefs, *output.Brief)
	}
	return nil
}

func finishScenario(environment *scenarioEnvironment) error {
	for suffix, item := range map[string]struct {
		agent     *mcpAgent
		workspace config.Workspace
	}{
		"a": {environment.agentA, environment.workspaceA},
		"b": {environment.agentB, environment.workspaceB},
	} {
		if _, err := item.agent.call(environment.ctx, "finish_work", map[string]any{
			"workspace_id": item.workspace.ID, "idempotency_key": "finish_" + suffix,
			"outcome": "Scripted scenario completed.", "summary": "Recorded bounded coordination evidence for scenario evaluation.",
		}); err != nil {
			return err
		}
	}
	return environment.waitForQueue(queueDrainTimeout)
}

func expectedFindings(id string, environment *scenarioEnvironment) []expectedFinding {
	targetA := environment.workspaceA.WorkstreamID
	targetReader := environment.readerWorkstream()
	targetBoth := environment.workspaceA.WorkstreamID + "+" + environment.workspaceB.WorkstreamID
	switch id {
	case "A", "C":
		return []expectedFinding{{Kind: "stale_assumption", TargetWorkstream: targetReader, Capability: capabilityContract, NamedEvidence: "Refresh"}}
	case "B":
		return []expectedFinding{{Kind: "redundant_work", TargetWorkstream: targetBoth, Capability: capabilitySemantic, NamedEvidence: "credential"}}
	case "D":
		return []expectedFinding{{Kind: "dependency_ready", TargetWorkstream: targetA, Capability: capabilityDependency, NamedEvidence: "session-api"}}
	case "E":
		return nil
	case "F":
		return []expectedFinding{{Kind: "direct_collision", TargetWorkstream: targetBoth, Capability: capabilityStructural, NamedEvidence: "shared/settings.ts"}}
	case "G":
		return []expectedFinding{{Kind: "stale_assumption", TargetWorkstream: targetReader, Capability: capabilitySemantic, NamedEvidence: "Refresh"}}
	default:
		return nil
	}
}

func scenarioAssertions(id string, environment *scenarioEnvironment, findings []actualFinding, observation scenarioObservation, required map[capability]bool) []assertionResult {
	contains := func(kind string, targets []string, evidence string) bool {
		for _, finding := range findings {
			if finding.Kind == kind && includesAll(finding.WorkstreamIDs, targets) && (evidence == "" || findingContains(finding, evidence)) {
				return true
			}
		}
		return false
	}
	interrupts := falseInterruptCount(id, findings, observation.briefs)
	switch id {
	case "A":
		return []assertionResult{
			taggedAssertion("stale-assumption finding names old/new Refresh signature and targets WS1's session", capabilityContract, contains("stale_assumption", []string{environment.readerWorkstream()}, "Refresh"), "expected exact symbol evidence for backend.Refresh", required),
			taggedAssertion("WS1 next turn receives the injected contract correction", capabilityInjection, strings.Contains(observation.injectedContext, "Refresh"), "expected correction injected at WS1's next turn boundary", required),
		}
	case "B":
		duplicate := contains("redundant_work", []string{environment.workspaceA.WorkstreamID, environment.workspaceB.WorkstreamID}, "")
		explained := false
		for _, finding := range findings {
			if finding.Kind == "redundant_work" && len(finding.Reason) > 12 && len(finding.Evidence) > 0 {
				explained = true
			}
		}
		return []assertionResult{
			taggedAssertion("semantic duplication targets both workstreams", capabilitySemantic, duplicate, "expected redundant_work-class finding", required),
			taggedAssertion("duplication includes an evidence-backed explanation", capabilitySemantic, explained, "expected reason and evidence", required),
			taggedAssertion("duplication never interrupts either agent mid-turn", capabilitySemantic, interrupts == 0, "expected dashboard plus next-turn delivery only", required),
		}
	case "C":
		return []assertionResult{
			taggedAssertion("contract drift appears within one publish cycle", capabilityContract, contains("stale_assumption", []string{environment.readerWorkstream()}, "Refresh"), "expected exact Refresh drift evidence", required),
			taggedAssertion("contract correction reaches WS1 at the next turn boundary", capabilityInjection, strings.Contains(observation.injectedContext, "Refresh"), "expected routed context", required),
			taggedAssertion("WS1 adjustment probe re-reads and changes intent", capabilityInjection, observation.adjustmentProbe, "expected intent revision after correction", required),
		}
	case "D":
		ready := contains("dependency_ready", []string{environment.workspaceA.WorkstreamID}, "session-api")
		wip := false
		for _, finding := range findings {
			text := strings.ToLower(finding.Reason + " " + findingText(finding))
			if strings.Contains(text, "wip") || strings.Contains(text, "unverified") || strings.Contains(text, "work-in-progress") {
				wip = true
			}
		}
		return []assertionResult{
			taggedAssertion("dependency-ready notice targets WS1 and names session-api", capabilityDependency, ready, "expected session API contract evidence", required),
			taggedAssertion("unverified checkpoint produces stable-but-WIP notice", capabilityDependency, wip, "expected uncertainty before verified readiness", required),
		}
	case "E":
		return []assertionResult{
			taggedAssertion("disjoint workstreams receive zero findings", capabilityStructural, len(findings) == 0, "any finding fails the silence scenario", required),
			taggedAssertion("disjoint workstreams receive no routed brief item", capabilityStructural, findingBriefItemCount(observation.briefs) == 0, "silence is a hard assertion", required),
			taggedAssertion("no context is injected into an independent session's turn", capabilityInjection, observation.injectedContext == "", "silence must hold on the delivery channel", required),
		}
	case "F":
		overlap := false
		for _, finding := range findings {
			if finding.Kind == "direct_collision" && includesAll(finding.WorkstreamIDs, []string{environment.workspaceA.WorkstreamID, environment.workspaceB.WorkstreamID}) && findingContains(finding, "shared/settings.ts") && severityRank(finding.Severity) <= severityRank("medium") {
				overlap = true
			}
		}
		return []assertionResult{
			taggedAssertion("same path produces a non-interrupt structural overlap", capabilityStructural, overlap, "expected medium-or-lower direct_collision on shared/settings.ts", required),
			taggedAssertion("unchanged exported contracts produce no stale assumption", capabilityContract, !hasFindingKind(findings, "stale_assumption"), "future contract analyzer must distinguish unrelated regions", required),
			taggedAssertion("same-file warning is not injected as an interrupt", capabilityInjection, interrupts == 0, "expected review-only delivery", required),
		}
	case "G":
		uncertain := false
		for _, finding := range findings {
			text := strings.ToLower(finding.Reason + " " + findingText(finding))
			if strings.Contains(text, "wip") || strings.Contains(text, "unverified") || strings.Contains(text, "uncertain") || strings.Contains(text, "work-in-progress") {
				uncertain = severityRank(finding.Severity) < severityRank("high")
			}
		}
		return []assertionResult{
			// WIP labeling is judgment-layer work: the implementation plan
			// gates scenario G under M4, not M2, so it carries the M4
			// capability tag rather than the deterministic contract tag.
			taggedAssertion("unverified contract change is labeled uncertain and lower severity", capabilitySemantic, uncertain, "expected WIP fidelity below scenarios A/C", required),
		}
	default:
		return nil
	}
}

func scenarioMetricValues(definition scenarioDefinition, environment *scenarioEnvironment, expected []expectedFinding, findings []actualFinding, observation scenarioObservation, elapsed time.Duration) scenarioMetrics {
	correct, routed := routingCounts(expected, findings, environment)
	rate := 0.0
	if routed > 0 {
		rate = float64(correct) / float64(routed)
	} else if definition.ExpectedRouting == "silence" {
		rate = 1
	}
	contextSufficient := len(expected) == 0
	for _, item := range expected {
		for _, finding := range findings {
			if finding.Kind == item.Kind && item.NamedEvidence != "" && findingContains(finding, item.NamedEvidence) {
				contextSufficient = true
			}
		}
	}
	return scenarioMetrics{
		CorrectTargetRate: rate, CorrectlyRouted: correct, AllRouted: routed,
		FalseInterruptCount: falseInterruptCount(definition.ID, findings, observation.briefs),
		SilenceHonored:      definition.ExpectedRouting == "silence" && len(findings) == 0 && findingBriefItemCount(observation.briefs) == 0,
		ContextSufficient:   contextSufficient, AdjustmentProbe: observation.adjustmentProbe,
		DeliveryMillis: observation.deliveryWait.Milliseconds(),
		WallTimeMillis: elapsed.Milliseconds(),
	}
}

func routingCounts(expected []expectedFinding, findings []actualFinding, environment *scenarioEnvironment) (int, int) {
	correct, routed := 0, 0
	for _, finding := range findings {
		for _, target := range finding.WorkstreamIDs {
			if target != environment.workspaceA.WorkstreamID && target != environment.workspaceB.WorkstreamID &&
				target != environment.readerWorkstreamA {
				continue
			}
			routed++
			for _, item := range expected {
				if item.Kind == finding.Kind && targetExpected(item.TargetWorkstream, target) {
					correct++
					break
				}
			}
		}
	}
	return correct, routed
}

func targetExpected(combined, target string) bool {
	for _, value := range strings.Split(combined, "+") {
		if value == target {
			return true
		}
	}
	return false
}

func annotateAdvisoryActions(findings []actualFinding, briefs []hosted.CoordinationBrief) []actualFinding {
	actions := map[string]map[string]bool{}
	for _, brief := range briefs {
		for _, item := range brief.Items {
			if actions[item.ID] == nil {
				actions[item.ID] = map[string]bool{}
			}
			actions[item.ID][item.AdvisoryAction] = true
		}
	}
	for index := range findings {
		for action := range actions[findings[index].ID] {
			findings[index].AdvisoryActions = append(findings[index].AdvisoryActions, action)
		}
		sort.Strings(findings[index].AdvisoryActions)
	}
	return findings
}

func actualRouting(findings []actualFinding, briefs []hosted.CoordinationBrief) string {
	if len(findings) == 0 && findingBriefItemCount(briefs) == 0 {
		return "silence"
	}
	// Severity says how much a finding matters, not how it is delivered. No
	// supported vendor exposes a mid-turn interrupt channel (ADR-033/046), so
	// even a high-severity correction reaches the agent at its next turn
	// boundary. Labeling that "interrupt" would claim a channel that does not
	// exist and would make an urgent-but-correctly-delivered finding look like
	// a routing failure.
	urgent := false
	for _, finding := range findings {
		if severityRank(finding.Severity) >= severityRank("high") {
			urgent = true
		}
		for _, action := range finding.AdvisoryActions {
			if action == "coordination_required" {
				urgent = true
			}
		}
	}
	if findingBriefItemCount(briefs) > 0 || urgent {
		return "next-turn"
	}
	return "dashboard-only"
}

func falseInterruptCount(id string, findings []actualFinding, briefs []hosted.CoordinationBrief) int {
	if id != "E" && id != "F" {
		return 0
	}
	interrupts := map[string]bool{}
	for _, finding := range findings {
		if severityRank(finding.Severity) >= severityRank("high") {
			interrupts[finding.ID] = true
		}
	}
	for _, brief := range briefs {
		for _, item := range brief.Items {
			if item.Kind == "finding" && item.AdvisoryAction == "coordination_required" {
				interrupts[item.ID] = true
			}
		}
	}
	return len(interrupts)
}

func findingBriefItemCount(briefs []hosted.CoordinationBrief) int {
	items := map[string]bool{}
	for _, brief := range briefs {
		for _, item := range brief.Items {
			if item.Kind == "finding" {
				items[item.ID] = true
			}
		}
	}
	return len(items)
}

func briefContains(briefs []hosted.CoordinationBrief, workstreamID, value string) bool {
	value = strings.ToLower(value)
	for _, brief := range briefs {
		if brief.WorkstreamID != workstreamID {
			continue
		}
		for _, item := range brief.Items {
			if strings.Contains(strings.ToLower(item.Text+" "+item.RelevanceReason), value) {
				return true
			}
		}
	}
	return false
}

func includesAll(values, targets []string) bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	for _, target := range targets {
		if !set[target] {
			return false
		}
	}
	return true
}

func findingContains(finding actualFinding, value string) bool {
	return strings.Contains(strings.ToLower(finding.Reason+" "+findingText(finding)), strings.ToLower(value))
}

func findingText(finding actualFinding) string {
	encoded, _ := json.Marshal(finding.Evidence)
	return string(encoded)
}

func hasFindingKind(findings []actualFinding, kind string) bool {
	for _, finding := range findings {
		if finding.Kind == kind {
			return true
		}
	}
	return false
}

func severityRank(severity string) int {
	return map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}[severity]
}
