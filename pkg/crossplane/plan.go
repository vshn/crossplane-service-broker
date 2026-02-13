package crossplane

import (
	"fmt"
	"strings"

	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// SLAPremium represents the string for the premium SLA
	SLAPremium = "premium"
	// SLAStandard represents the string for the standard SLA
	SLAStandard = "standard"
	// SizeXSmall represents the string for the xsmall plan size
	SizeXSmall = "xsmall"
	// SizeSmall represents the string for the small plan size
	SizeSmall = "small"
	// SizeMedium represents the string for the medium plan size
	SizeMedium = "medium"
	// SizeLarge represents the string for the large plan size
	SizeLarge = "large"
	// SizeXLarge represents the string for the xlarge plan size
	SizeXLarge = "xlarge"
)

// Plan is a wrapper around a Composition representing a service plan.
type Plan struct {
	Composition *xv1.Composition
	Labels      *Labels
	Metadata    string
	Tags        string
	Description string
	Schemas     string
}

// GVK returns the group, version, kind type for the composite type ref.
func (p Plan) GVK() (schema.GroupVersionKind, error) {
	groupVersion, err := schema.ParseGroupVersion(p.Composition.Spec.CompositeTypeRef.APIVersion)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return groupVersion.WithKind(p.Composition.Spec.CompositeTypeRef.Kind), nil
}

// ParsePlanUpdateRules parses the rules given as strings and return a PlanUpgradeChecker that implements them.
// It takes two rule strings.
// One that defines how plan SLAs can be changed and one that defines how plan sizes are allowed to be changed.
// Both are a `|` separated list of white-listed changes in the form of `$OLD_PLAN>$NEW_PLAN`.
//
// The example sizeRules `small>medium|medium>large`, will allow updating plans from small to medium and from medium to large and reject all other updates.
// The slaRules `standard>premium|premium>standard`, will allow switching between standard and premium SLA.
func ParsePlanUpdateRules(sizeRules, slaRules string) (PlanUpdateChecker, error) {
	var err error
	pc := PlanUpdateChecker{}

	pc.sizeRules, err = parsePlanUpdateRules(sizeRules)
	if err != nil {
		return pc, err
	}
	pc.slaRules, err = parsePlanUpdateRules(slaRules)
	if err != nil {
		return pc, err
	}

	return pc, nil
}

func parsePlanUpdateRules(ruleString string) ([]planUpdateRule, error) {
	rules := []planUpdateRule{}
	if ruleString == "" {
		return rules, nil
	}
	rs := strings.Split(ruleString, "|")
	for _, r := range rs {
		a := strings.Split(r, ">")
		if len(a) != 2 {
			return nil, fmt.Errorf("unable to parse rule: %s", r)
		}
		rules = append(rules, planUpdateRule{a[0], a[1]})
	}
	return rules, nil
}

// PlanUpdateChecker checks whether an update is valid
type PlanUpdateChecker struct {
	sizeRules []planUpdateRule
	slaRules  []planUpdateRule
}

type planUpdateRule struct {
	oldPlan string
	newPlan string
}

func (pc planUpdateRule) allowed(a, b string) bool {
	return a == pc.oldPlan && b == pc.newPlan
}

// AllowUpdate checks whether a plan upgrade from a to b is valid
func (pc PlanUpdateChecker) AllowUpdate(a, b Plan) bool {
	if a.Labels.ServiceID != b.Labels.ServiceID {
		// We do not allow changing services
		return false
	}

	if planSizeChanged(a, b) && planSLAChanged(a, b) {
		// We do not allow changing SLA and plan size at the same time
		return false
	}

	if planSizeChanged(a, b) {
		return pc.allowSizeUpdate(a, b)
	}
	if planSLAChanged(a, b) {
		return pc.allowSLAUpdate(a, b)
	}
	return true
}

func (pc PlanUpdateChecker) allowSizeUpdate(a, b Plan) bool {
	for _, c := range pc.sizeRules {
		if c.allowed(a.Labels.PlanSize, b.Labels.PlanSize) {
			return true
		}
	}
	return false
}
func (pc PlanUpdateChecker) allowSLAUpdate(a, b Plan) bool {
	for _, c := range pc.slaRules {
		if c.allowed(a.Labels.SLA, b.Labels.SLA) {
			return true
		}
	}
	return false
}

func planSizeChanged(a, b Plan) bool {
	return a.Labels != nil && b.Labels != nil && a.Labels.PlanSize != b.Labels.PlanSize
}

func planSLAChanged(a, b Plan) bool {
	return a.Labels != nil && b.Labels != nil && a.Labels.SLA != b.Labels.SLA
}

func newPlan(c xv1.Composition) (*Plan, error) {
	l, err := parseLabels(c.Labels)
	if err != nil {
		return nil, err
	}
	return &Plan{
		Composition: &c,
		Labels:      l,
		Metadata:    c.Annotations[MetadataAnnotation],
		Tags:        c.Annotations[TagsAnnotation],
		Description: c.Annotations[DescriptionAnnotation],
		Schemas:     c.Annotations[SchemaAnnotation],
	}, nil
}
