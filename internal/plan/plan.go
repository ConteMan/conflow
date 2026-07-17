// Package plan builds and persists immutable plan read models. It depends on
// no HTTP or provider SDK, keeping the compilation/diff boundary reusable.
package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ConteMan/conflow/internal/entities"
	"github.com/ConteMan/conflow/internal/remote"
)

const DefaultTTL = 15 * time.Minute

type SemanticChange struct {
	NodeID                 string   `json:"node_id"`
	ChangeKind             string   `json:"change_kind"`
	Summary                string   `json:"summary"`
	DirectEntityRef        string   `json:"direct_entity_ref,omitempty"`
	FieldPath              string   `json:"field_path,omitempty"`
	BeforeSummary          string   `json:"before_summary,omitempty"`
	AfterSummary           string   `json:"after_summary,omitempty"`
	AffectedEntityIDs      []string `json:"affected_entity_ids"`
	AffectedEntityNodeIDs  []string `json:"affected_entity_node_ids"`
	RemoteParameterNodeIDs []string `json:"remote_parameter_node_ids"`
}
type AffectedEntity struct {
	NodeID                    string   `json:"node_id"`
	EntityRef                 string   `json:"entity_ref"`
	EntityType                string   `json:"entity_type"`
	EntityID                  string   `json:"entity_id"`
	ImpactKind                string   `json:"impact_kind"`
	CausedBySemanticChangeIDs []string `json:"caused_by_semantic_change_ids"`
}
type RemoteParameterChange struct {
	NodeID                    string   `json:"node_id"`
	ProjectionID              string   `json:"projection_id,omitempty"`
	ParameterKey              string   `json:"parameter_key"`
	ChangeKind                string   `json:"change_kind"`
	BeforeSummary             string   `json:"before_summary,omitempty"`
	AfterSummary              string   `json:"after_summary,omitempty"`
	Managed                   bool     `json:"managed"`
	CausedBySemanticChangeIDs []string `json:"caused_by_semantic_change_ids"`
	AffectedEntityNodeIDs     []string `json:"affected_entity_node_ids"`
}
type ArtifactMetadata struct {
	ArtifactName  string `json:"artifact_name"`
	MediaType     string `json:"media_type"`
	ContentDigest string `json:"content_digest"`
	SizeBytes     int64  `json:"size_bytes"`
	Sensitive     bool   `json:"sensitive"`
	Available     bool   `json:"available"`
}
type RiskItem struct {
	RiskItemID              string   `json:"risk_item_id"`
	Severity                string   `json:"severity"`
	ReasonCode              string   `json:"reason_code"`
	Summary                 string   `json:"summary"`
	EntityRef               string   `json:"entity_ref,omitempty"`
	SemanticChangeIDs       []string `json:"semantic_change_ids"`
	RemoteParameterNodeIDs  []string `json:"remote_parameter_node_ids"`
	AcknowledgementRequired bool     `json:"acknowledgement_required"`
}
type BlockingReason struct {
	ReasonCode string `json:"reason_code"`
	Summary    string `json:"summary"`
	RiskItemID string `json:"risk_item_id,omitempty"`
	NodeID     string `json:"node_id,omitempty"`
}
type ConfirmationRequirements struct {
	RequiresAcknowledgement  bool     `json:"requires_acknowledgement"`
	EnvironmentIDRequirement string   `json:"environment_id_requirement"`
	RequiredRiskItemIDs      []string `json:"required_risk_item_ids"`
	PolicySource             string   `json:"policy_source"`
}
type Invalidation struct {
	Code    string `json:"code"`
	Tier    string `json:"tier"`
	Message string `json:"message"`
}
type Plan struct {
	PlanID                   string                   `json:"plan_id"`
	EnvironmentID            string                   `json:"environment_id"`
	PackRef                  string                   `json:"pack_ref"`
	Status                   string                   `json:"status"`
	SnapshotToken            string                   `json:"snapshot_token"`
	DraftRevision            uint64                   `json:"draft_revision"`
	SourceDigest             string                   `json:"source_digest"`
	RemoteETag               *string                  `json:"remote_etag"`
	CreatedAt                time.Time                `json:"created_at"`
	ExpiresAt                time.Time                `json:"expires_at"`
	InvalidationReason       string                   `json:"invalidation_reason,omitempty"`
	Invalidation             *Invalidation            `json:"invalidation,omitempty"`
	ContentDigest            string                   `json:"content_digest"`
	RemoteSnapshot           remote.Snapshot          `json:"remote_snapshot"`
	SemanticChanges          []SemanticChange         `json:"semantic_changes"`
	AffectedEntities         []AffectedEntity         `json:"affected_entities"`
	RemoteParameterChanges   []RemoteParameterChange  `json:"remote_parameter_changes"`
	ArtifactMetadata         []ArtifactMetadata       `json:"artifact_metadata"`
	Severity                 string                   `json:"severity"`
	RiskItems                []RiskItem               `json:"risk_items"`
	BlockingReasons          []BlockingReason         `json:"blocking_reasons"`
	ConfirmationRequirements ConfirmationRequirements `json:"confirmation_requirements"`
	packSchemaVersion        uint64
}
type Input struct {
	EnvironmentID, EnvironmentKind, PackRef, SourceDigest string
	DraftRevision                                         uint64
	PackSchemaVersion                                     uint64
	Desired, Baseline                                     map[string]any
	// BaseLayer is the resolved baseline before an environment override. It
	// preserves a direct override even when final effective value equals source.
	BaseLayer             map[string]any
	RemoteSnapshot        remote.Snapshot
	ValidationReady       bool
	ProductionLowRiskMode string
	Now                   time.Time
	TTL                   time.Duration
	scope                 string
}
type BuildResult struct {
	Plan      Plan
	Artifacts map[string][]byte
}

var ErrNotFound = errors.New("plan not found")

func Build(in Input) (BuildResult, error) {
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}
	if in.TTL == 0 {
		in.TTL = DefaultTTL
	}
	p := Plan{PlanID: "plan_" + id(in.EnvironmentID, fmt.Sprintf("%d", in.Now.UnixNano())), EnvironmentID: in.EnvironmentID, PackRef: in.PackRef, Status: "ready", DraftRevision: in.DraftRevision, SourceDigest: in.SourceDigest, CreatedAt: in.Now.UTC(), ExpiresAt: in.Now.UTC().Add(in.TTL), RemoteSnapshot: in.RemoteSnapshot, SemanticChanges: []SemanticChange{}, AffectedEntities: []AffectedEntity{}, RemoteParameterChanges: []RemoteParameterChange{}, ArtifactMetadata: []ArtifactMetadata{}, RiskItems: []RiskItem{}, BlockingReasons: []BlockingReason{}, packSchemaVersion: in.PackSchemaVersion}
	if in.RemoteSnapshot.Status != "available" {
		p.Status = "preview_only"
	} else {
		etag := in.RemoteSnapshot.RemoteETag
		p.RemoteETag = &etag
	}
	p.SemanticChanges = semanticChanges(in, &p)
	p.RiskItems = risks(in, p.SemanticChanges, p.RemoteParameterChanges)
	p.BlockingReasons = blockingReasons(p.RiskItems)
	p.Severity = highestSeverity(p.RiskItems, p.BlockingReasons)
	p.ConfirmationRequirements = confirmations(in, p.RiskItems, p.BlockingReasons)
	// This is deliberately a one-way token. It carries all preconditions that
	// identify the build input, but it never exposes them to the client.
	p.SnapshotToken = "pst_" + hash(canonical(map[string]any{"draft_revision": p.DraftRevision, "source_digest": p.SourceDigest, "remote_etag": p.RemoteETag, "remote_status": in.RemoteSnapshot.Status, "unavailable_reason": in.RemoteSnapshot.UnavailableReason, "pack_ref": in.PackRef, "desired": in.Desired}))[:24]
	artifacts := map[string][]byte{}
	// Artifact digest is over the redacted read model before metadata is added,
	// avoiding a self-referential digest while keeping all content deterministic.
	review, err := json.MarshalIndent(contentForDigest(p), "", "  ")
	if err != nil {
		return BuildResult{}, err
	}
	review = append(review, '\n')
	artifacts["review.json"] = review
	artifacts["review.md"] = []byte(markdown(p))
	if p.Status == "ready" {
		provider, err := json.MarshalIndent(redact(in.Desired), "", "  ")
		if err != nil {
			return BuildResult{}, err
		}
		artifacts["provider-input.json"] = append(provider, '\n')
	}
	for name, content := range artifacts {
		media := "application/json"
		if strings.HasSuffix(name, ".md") {
			media = "text/markdown; charset=utf-8"
		}
		p.ArtifactMetadata = append(p.ArtifactMetadata, ArtifactMetadata{ArtifactName: name, MediaType: media, ContentDigest: "sha256:" + hash(content), SizeBytes: int64(len(content)), Sensitive: false, Available: true})
	}
	sort.Slice(p.ArtifactMetadata, func(i, j int) bool { return p.ArtifactMetadata[i].ArtifactName < p.ArtifactMetadata[j].ArtifactName })
	p.ContentDigest = "sha256:" + hash(canonical(contentForDigest(p)))
	return BuildResult{Plan: p, Artifacts: artifacts}, nil
}

func semanticChanges(in Input, p *Plan) []SemanticChange {
	changes := []SemanticChange{}
	compiledV2 := map[string]any(nil)
	if in.PackRef == "mobile-ad-monetization/v2" {
		compiledV2 = compileV2Parameters(in.Desired, in.EnvironmentID)
	}
	scope := in.scope
	if scope == "" {
		scope = "baseline"
	}
	comparison := in.Desired
	if in.BaseLayer != nil {
		comparison = in.BaseLayer
	}
	collections := map[string]string{"frequency_policies": "frequency_policy", "feature_switches": "feature_switch", "placements": "placement", "unit_bindings": "unit_binding"}
	if in.PackRef == "mobile-ad-monetization/v2" {
		collections = map[string]string{
			"custom_parameters":     "custom_parameter",
			"remote_config_layouts": "remote_config_layout",
			"network_settings":      "network_settings",
			"frequency_policies":    "frequency_policy",
			"placements":            "placement",
			"unit_bindings":         "unit_binding",
			"feature_switches":      "feature_switch",
		}
	}
	for collection, entityType := range collections {
		oldRecords := records(in.Baseline[collection])
		newRecords := records(comparison[collection])
		ids := map[string]bool{}
		for k := range oldRecords {
			ids[k] = true
		}
		for k := range newRecords {
			ids[k] = true
		}
		ordered := keys(ids)
		for _, entityID := range ordered {
			before, beforeOK := oldRecords[entityID]
			after, afterOK := newRecords[entityID]
			if entityType == "custom_parameter" {
				if change := customParameterSemanticChange(in, scope, entityID, before, beforeOK, after, afterOK); change != nil {
					if len(change.RemoteParameterNodeIDs) > 0 {
						if remoteChange, changed := customParameterRemoteChange(in, scope, *change, before, beforeOK, after, afterOK); changed {
							p.RemoteParameterChanges = append(p.RemoteParameterChanges, remoteChange)
						} else {
							change.RemoteParameterNodeIDs = []string{}
						}
					}
					changes = append(changes, *change)
				}
				continue
			}
			fields := map[string]bool{}
			for k := range before.Fields {
				fields[k] = true
			}
			for k := range after.Fields {
				fields[k] = true
			}
			for _, field := range keys(fields) {
				bv, bok := before.Fields[field]
				av, aok := after.Fields[field]
				if bok == aok && equal(bv, av) {
					continue
				}
				node := "node_" + id("semantic", scope, entityType, entityID, field)
				ref := entityRef(in.PackRef, entityType, entityID)
				changeKind := "updated"
				if !aok {
					changeKind = "deleted"
				} else if !bok {
					changeKind = "added"
				}
				c := SemanticChange{NodeID: node, ChangeKind: changeKind, Summary: fmt.Sprintf("%s %s changed", entityType, entityID), DirectEntityRef: ref, FieldPath: "/" + field, BeforeSummary: summary(bv), AfterSummary: summary(av), AffectedEntityIDs: []string{}, AffectedEntityNodeIDs: []string{}, RemoteParameterNodeIDs: []string{}}
				if entityType == "frequency_policy" {
					for _, placementID := range placementsUsing(in.Desired, entityID) {
						entityNode := "node_" + id("entity", "placement", placementID)
						p.AffectedEntities = append(p.AffectedEntities, AffectedEntity{NodeID: entityNode, EntityRef: entityRef(in.PackRef, "placement", placementID), EntityType: "placement", EntityID: placementID, ImpactKind: "referenced", CausedBySemanticChangeIDs: []string{node}})
						c.AffectedEntityIDs = append(c.AffectedEntityIDs, entityRef(in.PackRef, "placement", placementID))
						c.AffectedEntityNodeIDs = append(c.AffectedEntityNodeIDs, entityNode)
					}
				}
				if in.RemoteSnapshot.Status == "available" {
					remoteNode := "node_" + id("remote", scope, entityType, entityID, field)
					key := parameterKey(entityType, entityID, field)
					remoteBefore, remoteExists := in.RemoteSnapshot.Parameters[key]
					remoteAfter := av
					remoteChangeKind := changeKind
					skipRemoteChange := false
					if in.PackRef == "mobile-ad-monetization/v2" {
						key = affectedParameterKey(in.PackRef, entityType, entityID, in.Desired)
						remoteBefore, remoteExists = in.RemoteSnapshot.Parameters[key]
						if value, exists := compiledV2[key]; exists {
							remoteAfter = value
							if remoteValuesEqual(remoteBefore, remoteAfter) {
								skipRemoteChange = true
							}
							if !skipRemoteChange && remoteChangeKind == "deleted" {
								remoteChangeKind = "updated"
							} else if !skipRemoteChange && remoteExists {
								remoteChangeKind = "updated"
							} else if !skipRemoteChange {
								remoteChangeKind = "added"
							}
						}
					} else if !remoteExists {
						remoteBefore = bv
					}
					if !skipRemoteChange {
						p.RemoteParameterChanges = append(p.RemoteParameterChanges, RemoteParameterChange{NodeID: remoteNode, ProjectionID: "rvp_" + id(in.EnvironmentID, key), ParameterKey: key, ChangeKind: remoteChangeKind, BeforeSummary: summary(remoteBefore), AfterSummary: summary(remoteAfter), Managed: true, CausedBySemanticChangeIDs: []string{node}, AffectedEntityNodeIDs: append([]string{}, c.AffectedEntityNodeIDs...)})
						c.RemoteParameterNodeIDs = []string{remoteNode}
					}
				}
				changes = append(changes, c)
			}
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].NodeID < changes[j].NodeID })
	sort.Slice(p.AffectedEntities, func(i, j int) bool { return p.AffectedEntities[i].NodeID < p.AffectedEntities[j].NodeID })
	sort.Slice(p.RemoteParameterChanges, func(i, j int) bool { return p.RemoteParameterChanges[i].NodeID < p.RemoteParameterChanges[j].NodeID })
	if in.BaseLayer != nil {
		override := in
		override.Baseline, override.BaseLayer = in.BaseLayer, nil
		override.scope = "override"
		changes = append(changes, semanticChanges(override, p)...)
		sort.Slice(changes, func(i, j int) bool { return changes[i].NodeID < changes[j].NodeID })
	}
	return changes
}

func customParameterSemanticChange(in Input, scope, entityID string, before entities.Record, beforeOK bool, after entities.Record, afterOK bool) *SemanticChange {
	if beforeOK == afterOK && equal(before.Fields, after.Fields) {
		return nil
	}
	changeKind := "updated"
	if !afterOK {
		changeKind = "deleted"
	} else if !beforeOK {
		changeKind = "added"
	}
	keyRecord := after
	if !afterOK {
		keyRecord = before
	}
	key, _ := keyRecord.Fields["key"].(string)
	remoteChanged := key != "" && (!beforeOK || !afterOK || !equal(before.Fields["key"], after.Fields["key"]) || !equal(before.Fields["value_type"], after.Fields["value_type"]) || !equal(before.Fields["value"], after.Fields["value"]))
	fieldPath := "/description"
	beforeSummary, afterSummary := summary(before.Fields["description"]), summary(after.Fields["description"])
	if remoteChanged {
		fieldPath = "/value"
		beforeSummary, afterSummary = summary(before.Fields["value"]), summary(after.Fields["value"])
	}
	change := &SemanticChange{
		NodeID:                 "node_" + id("semantic", scope, "custom_parameter", entityID, "value"),
		ChangeKind:             changeKind,
		Summary:                fmt.Sprintf("custom_parameter %s changed", entityID),
		DirectEntityRef:        entityRef(in.PackRef, "custom_parameter", entityID),
		FieldPath:              fieldPath,
		BeforeSummary:          beforeSummary,
		AfterSummary:           afterSummary,
		AffectedEntityIDs:      []string{},
		AffectedEntityNodeIDs:  []string{},
		RemoteParameterNodeIDs: []string{},
	}
	if !remoteChanged || in.RemoteSnapshot.Status != "available" {
		return change
	}
	change.RemoteParameterNodeIDs = []string{"node_" + id("remote", scope, "custom_parameter", entityID, "value")}
	return change
}

func customParameterRemoteChange(in Input, scope string, semantic SemanticChange, before entities.Record, beforeOK bool, after entities.Record, afterOK bool) (RemoteParameterChange, bool) {
	keyRecord := after
	if !afterOK {
		keyRecord = before
	}
	key, _ := keyRecord.Fields["key"].(string)
	beforeValue := before.Fields["value"]
	if remoteValue, exists := in.RemoteSnapshot.Parameters[key]; exists {
		beforeValue = remoteValue
	}
	afterValue := any(nil)
	if afterOK {
		afterValue = compileV2Parameters(in.Desired, in.EnvironmentID)[key]
	}
	_, remoteExists := in.RemoteSnapshot.Parameters[key]
	if afterOK && remoteExists && remoteValuesEqual(beforeValue, afterValue) {
		return RemoteParameterChange{}, false
	}
	changeKind := semantic.ChangeKind
	if changeKind != "deleted" {
		if remoteExists {
			changeKind = "updated"
		} else {
			changeKind = "added"
		}
	}
	return RemoteParameterChange{NodeID: semantic.RemoteParameterNodeIDs[0], ProjectionID: "rvp_" + id(in.EnvironmentID, key), ParameterKey: key, ChangeKind: changeKind, BeforeSummary: summary(beforeValue), AfterSummary: summary(afterValue), Managed: true, CausedBySemanticChangeIDs: []string{semantic.NodeID}, AffectedEntityNodeIDs: []string{}}, true
}

func risks(in Input, changes []SemanticChange, remoteChanges []RemoteParameterChange) []RiskItem {
	result := []RiskItem{}
	for _, c := range changes {
		if in.PackRef == "mobile-ad-monetization/v2" && len(c.RemoteParameterNodeIDs) == 0 {
			continue
		}
		if in.PackRef != "mobile-ad-monetization/v2" && strings.Contains(c.DirectEntityRef, ":frequency_policy:") && c.FieldPath == "/cooldown_ms" {
			result = append(result, RiskItem{RiskItemID: "risk_" + id("shared_frequency", c.NodeID), Severity: "high", ReasonCode: "shared_frequency_policy_relaxed", Summary: "共享频控已放宽", EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
		}
		if in.PackRef != "mobile-ad-monetization/v2" && in.EnvironmentKind == "production" && strings.Contains(c.DirectEntityRef, ":feature_switch:") {
			result = append(result, RiskItem{RiskItemID: "risk_" + id("switch", c.NodeID), Severity: "high", ReasonCode: "global_feature_switch_changed", Summary: "全局功能开关已变更", EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
		}
		if in.PackRef != "mobile-ad-monetization/v2" && in.EnvironmentKind == "production" && strings.Contains(c.DirectEntityRef, ":placement:") && c.FieldPath == "/network_mode" {
			result = append(result, RiskItem{RiskItemID: "risk_" + id("network_mode", c.NodeID), Severity: "high", ReasonCode: "production_network_mode_changed", Summary: "生产环境网络模式已变更", EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
		}
		if in.PackRef != "mobile-ad-monetization/v2" && strings.Contains(c.DirectEntityRef, ":unit_binding:") {
			result = append(result, RiskItem{RiskItemID: "risk_" + id("unit_binding", c.NodeID), Severity: "medium", ReasonCode: "unit_binding_changed", Summary: "广告单元绑定已变更", EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
		}
		if in.PackRef == "mobile-ad-monetization/v2" {
			switch {
			case strings.Contains(c.DirectEntityRef, ":custom_parameter:") && len(c.RemoteParameterNodeIDs) > 0:
				severity, code, summary := "medium", "custom_parameter_changed", "自定义参数已变更"
				if c.ChangeKind == "deleted" {
					severity, code, summary = "high", "custom_parameter_deleted", "自定义参数将从远端移除"
				} else if c.ChangeKind == "added" {
					for _, remoteChange := range remoteChanges {
						if remoteChange.CausedBySemanticChangeIDs[0] == c.NodeID {
							if _, exists := in.RemoteSnapshot.Parameters[remoteChange.ParameterKey]; exists {
								severity, code, summary = "high", "custom_parameter_adopted", "发布后该参数由 Conflow 接管，远端当前值将被覆盖"
							}
							break
						}
					}
				}
				result = append(result, RiskItem{RiskItemID: "risk_" + id(code, c.NodeID), Severity: severity, ReasonCode: code, Summary: summary, EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
			case strings.Contains(c.DirectEntityRef, ":frequency_policy:"):
				result = append(result, RiskItem{RiskItemID: "risk_" + id("v2_frequency_policy", c.NodeID), Severity: "high", ReasonCode: "frequency_policy_changed", Summary: "频控策略已变更", EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
			case strings.Contains(c.DirectEntityRef, ":feature_switch:") && c.FieldPath == "/default_value":
				result = append(result, RiskItem{RiskItemID: "risk_" + id("v2_switch", c.NodeID), Severity: "high", ReasonCode: "global_feature_switch_changed", Summary: "全局功能开关默认值已变更", EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
			case in.EnvironmentKind == "production" && strings.Contains(c.DirectEntityRef, ":network_settings:"):
				result = append(result, RiskItem{RiskItemID: "risk_" + id("v2_network_settings", c.NodeID), Severity: "high", ReasonCode: "production_network_settings_changed", Summary: "生产网络设置已变更", EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
			case strings.Contains(c.DirectEntityRef, ":unit_binding:"):
				result = append(result, RiskItem{RiskItemID: "risk_" + id("v2_unit_binding", c.NodeID), Severity: "medium", ReasonCode: "unit_binding_changed", Summary: "广告单元绑定已变更", EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
			case strings.Contains(c.DirectEntityRef, ":remote_config_layout:") && strings.HasSuffix(c.FieldPath, "_parameter_key"):
				result = append(result, RiskItem{RiskItemID: "risk_" + id("v2_layout_key", c.NodeID), Severity: "high", ReasonCode: "remote_parameter_key_changed", Summary: "远端参数键已变更", EntityRef: c.DirectEntityRef, SemanticChangeIDs: []string{c.NodeID}, RemoteParameterNodeIDs: c.RemoteParameterNodeIDs, AcknowledgementRequired: true})
			}
		}
	}
	changesByID := make(map[string]SemanticChange, len(changes))
	for _, c := range changes {
		changesByID[c.NodeID] = c
	}
	for _, remoteChange := range remoteChanges {
		if remoteChange.ChangeKind != "deleted" || !remoteChange.Managed {
			continue
		}
		if _, exists := in.RemoteSnapshot.Parameters[remoteChange.ParameterKey]; !exists {
			continue
		}
		var entityRef string
		if len(remoteChange.CausedBySemanticChangeIDs) > 0 {
			entityRef = changesByID[remoteChange.CausedBySemanticChangeIDs[0]].DirectEntityRef
		}
		if strings.Contains(entityRef, ":custom_parameter:") {
			continue
		}
		result = append(result, RiskItem{RiskItemID: "risk_" + id("managed_parameter_deleted", remoteChange.NodeID), Severity: "blocking", ReasonCode: "managed_parameter_deleted", Summary: "受管远端参数将被删除", EntityRef: entityRef, SemanticChangeIDs: append([]string{}, remoteChange.CausedBySemanticChangeIDs...), RemoteParameterNodeIDs: []string{remoteChange.NodeID}})
	}
	if !in.ValidationReady {
		result = append(result, RiskItem{RiskItemID: "risk_" + id("validation_not_ready", in.EnvironmentID), Severity: "blocking", ReasonCode: "validation_not_ready", Summary: "完整校验尚未就绪"})
	}
	if in.RemoteSnapshot.Status != "available" {
		result = append(result, RiskItem{RiskItemID: "risk_" + id("remote_snapshot_unavailable", in.EnvironmentID, string(in.RemoteSnapshot.UnavailableReason)), Severity: "blocking", ReasonCode: "remote_snapshot_unavailable", Summary: "远端快照不可用"})
	}
	if in.RemoteSnapshot.UnavailableReason == remote.SnapshotMissing {
		result = append(result, RiskItem{RiskItemID: "risk_" + id("remote_baseline_missing", in.EnvironmentID), Severity: "blocking", ReasonCode: "remote_baseline_missing", Summary: "远端基线缺失"})
	}
	if in.RemoteSnapshot.Summary != nil && in.RemoteSnapshot.Summary.HasUnmodeledConditions {
		result = append(result, RiskItem{RiskItemID: "risk_" + id("unmodeled_remote_condition", in.EnvironmentID, in.RemoteSnapshot.RemoteETag), Severity: "blocking", ReasonCode: "unmodeled_remote_condition", Summary: "远端存在尚未建模的条件值"})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RiskItemID < result[j].RiskItemID })
	return result
}
func blockingReasons(items []RiskItem) []BlockingReason {
	result := []BlockingReason{}
	for _, item := range items {
		if item.Severity == "blocking" {
			result = append(result, BlockingReason{ReasonCode: item.ReasonCode, Summary: item.Summary, RiskItemID: item.RiskItemID})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ReasonCode == result[j].ReasonCode {
			return result[i].RiskItemID < result[j].RiskItemID
		}
		return result[i].ReasonCode < result[j].ReasonCode
	})
	return result
}
func confirmations(in Input, risks []RiskItem, blocking []BlockingReason) ConfirmationRequirements {
	required := []string{}
	for _, r := range risks {
		if r.AcknowledgementRequired && r.Severity != "blocking" {
			required = append(required, r.RiskItemID)
		}
	}
	env := "not_required"
	if in.EnvironmentKind == "production" && len(required) == 0 && in.ProductionLowRiskMode != "acknowledgement" {
		env = "required"
	}
	// Every release remains an explicit acknowledgement. The project policy only
	// relaxes the extra production environment-ID input for a low-risk Plan.
	return ConfirmationRequirements{RequiresAcknowledgement: true, EnvironmentIDRequirement: env, RequiredRiskItemIDs: required, PolicySource: "project.release_confirmation_policy"}
}
func highestSeverity(risks []RiskItem, blocking []BlockingReason) string {
	if len(blocking) > 0 {
		return "blocking"
	}
	result := "low"
	for _, r := range risks {
		if rank(r.Severity) > rank(result) {
			result = r.Severity
		}
	}
	return result
}
func rank(v string) int {
	switch v {
	case "blocking":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}
func records(v any) map[string]entities.Record {
	out := map[string]entities.Record{}
	for _, record := range entities.Records(map[string]any{"records": v}, "records") {
		out[record.ID] = record
	}
	return out
}
func placementsUsing(desired map[string]any, policy string) []string {
	out := []string{}
	for id, r := range records(desired["placements"]) {
		if r.Fields["frequency_policy_id"] == policy {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
func entityRef(pack, typ, id string) string { return "entity:" + pack + ":" + typ + ":" + id }
func parameterKey(typ, id, field string) string {
	if typ == "frequency_policy" && field == "cooldown_ms" {
		return "ad_frequency_" + id
	}
	return typ + "_" + id + "_" + field
}
func affectedParameterKey(packRef, entityType, entityID string, desired map[string]any) string {
	if packRef != "mobile-ad-monetization/v2" {
		return parameterKey(entityType, entityID, "")
	}
	layout, found := records(desired["remote_config_layouts"])["default"]
	if !found {
		return "remote_config_layout_changed"
	}
	key := func(field string) string {
		value, _ := layout.Fields[field].(string)
		return value
	}
	switch entityType {
	case "feature_switch":
		if featureSwitch, found := records(desired["feature_switches"])[entityID]; found {
			if value, ok := featureSwitch.Fields["key"].(string); ok && value != "" {
				return value
			}
		}
	case "frequency_policy":
		return key("frequency_policies_parameter_key")
	case "placement", "unit_binding":
		return key("placements_parameter_key")
	case "network_settings":
		return key("active_network_parameter_key")
	case "remote_config_layout":
		return "remote_config_layout_changed"
	}
	return entityType + "_" + entityID
}
func summary(v any) string {
	if f, ok := v.(float64); ok && strings.Contains(fmt.Sprint(f), "000") {
		return fmt.Sprintf("%.0f seconds", f/1000)
	}
	if i, ok := v.(int); ok && i%1000 == 0 {
		return fmt.Sprintf("%d seconds", i/1000)
	}
	b, _ := json.Marshal(redact(v))
	return string(b)
}
func equal(a, b any) bool { return string(canonical(a)) == string(canonical(b)) }
func remoteValuesEqual(a, b any) bool {
	return equal(normalizeJSONValue(a), normalizeJSONValue(b))
}
func normalizeJSONValue(value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	var parsed any
	if json.Unmarshal([]byte(text), &parsed) != nil {
		return value
	}
	return parsed
}
func keys[V any](m map[string]V) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
func id(parts ...string) string { return hash([]byte(strings.Join(parts, "|")))[:16] }
func hash(b []byte) string      { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func canonical(v any) []byte    { b, _ := json.Marshal(v); return b }
func redact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		r := map[string]any{}
		for k, v := range x {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "token") || strings.Contains(lk, "credential") || strings.Contains(lk, "secret") {
				r[k] = "[redacted]"
			} else {
				r[k] = redact(v)
			}
		}
		return r
	case []any:
		r := make([]any, len(x))
		for i := range x {
			r[i] = redact(x[i])
		}
		return r
	default:
		return v
	}
}
func contentForDigest(p Plan) any {
	p.PlanID = ""
	p.CreatedAt = time.Time{}
	p.ExpiresAt = time.Time{}
	p.SnapshotToken = ""
	return p
}
func markdown(p Plan) string {
	return fmt.Sprintf("# Plan Review\n\nStatus: %s\n\nSemantic changes: %d\n", p.Status, len(p.SemanticChanges))
}

type Store struct {
	mu    sync.RWMutex
	root  string
	plans map[string]Plan
}

type storedPlan struct {
	Plan
	PackSchemaVersion uint64 `json:"pack_schema_version"`
}

func marshalPlan(p Plan) ([]byte, error) {
	return json.MarshalIndent(storedPlan{Plan: p, PackSchemaVersion: p.packSchemaVersion}, "", "  ")
}

func unmarshalPlan(data []byte) (Plan, error) {
	var stored storedPlan
	if err := json.Unmarshal(data, &stored); err != nil {
		return Plan{}, err
	}
	stored.Plan.packSchemaVersion = stored.PackSchemaVersion
	return stored.Plan, nil
}

func (p Plan) PackSchemaVersion() uint64 { return p.packSchemaVersion }

func Open(root string) (*Store, error) {
	s := &Store{root: root, plans: map[string]Plan{}}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name(), "plan.json"))
		if err != nil {
			return nil, err
		}
		p, err := unmarshalPlan(b)
		if err != nil {
			return nil, err
		}
		s.plans[p.PlanID] = p
	}
	return s, nil
}
func (s *Store) Save(result BuildResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.root, result.Plan.PlanID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for name, b := range result.Artifacts {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			return err
		}
	}
	b, err := marshalPlan(result.Plan)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "plan.json"), append(b, '\n'), 0o600); err != nil {
		return err
	}
	s.plans[result.Plan.PlanID] = result.Plan
	return nil
}
func (s *Store) Get(id string) (Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[id]
	if !ok {
		return Plan{}, ErrNotFound
	}
	return p, nil
}
func (s *Store) Update(p Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plans[p.PlanID]; !ok {
		return ErrNotFound
	}
	b, err := marshalPlan(p)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.root, p.PlanID, "plan.json"), append(b, '\n'), 0o600); err != nil {
		return err
	}
	s.plans[p.PlanID] = p
	return nil
}
func (s *Store) Artifact(id, name string) ([]byte, ArtifactMetadata, error) {
	p, err := s.Get(id)
	if err != nil {
		return nil, ArtifactMetadata{}, err
	}
	for _, m := range p.ArtifactMetadata {
		if m.ArtifactName == name && m.Available {
			b, e := os.ReadFile(filepath.Join(s.root, id, name))
			return b, m, e
		}
	}
	return nil, ArtifactMetadata{}, ErrNotFound
}
