package launchdarkly

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	ldapi "github.com/launchdarkly/api-client-go/v7"
)

func segmentRulesSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				CLAUSES: clauseSchema(),
				WEIGHT: {
					Type:             schema.TypeInt,
					Elem:             &schema.Schema{Type: schema.TypeInt},
					Optional:         true,
					ValidateDiagFunc: validation.ToDiagFunc(validation.IntBetween(0, 100000)),
					Description:      "The integer weight of the rule (between 0 and 100000).",
				},
				BUCKET_BY: {
					Type:        schema.TypeString,
					Elem:        &schema.Schema{Type: schema.TypeString},
					Optional:    true,
					Description: "The attribute by which to group users together.",
				},
			},
		},
	}
}

func segmentRulesFromResourceData(d *schema.ResourceData, metaRaw interface{}) ([]ldapi.UserSegmentRule, error) {
	schemaRules := d.Get(RULES).([]interface{})
	rules := make([]ldapi.UserSegmentRule, len(schemaRules))
	for i, rule := range schemaRules {
		v, err := segmentRuleFromResourceData(rule)
		if err != nil {
			return rules, err
		}
		rules[i] = v
	}

	return rules, nil
}

func segmentRuleFromResourceData(val interface{}) (ldapi.UserSegmentRule, error) {
	ruleMap := val.(map[string]interface{})
	weight := int32(ruleMap[WEIGHT].(int))
	bucketBy := ruleMap[BUCKET_BY].(string)
	r := ldapi.UserSegmentRule{
		Weight:   &weight,
		BucketBy: &bucketBy,
	}
	for _, c := range ruleMap[CLAUSES].([]interface{}) {
		clause, err := clauseFromResourceData(c)
		if err != nil {
			return r, err
		}
		r.Clauses = append(r.Clauses, clause)
	}

	return r, nil
}

func segmentRulesToResourceData(rules []ldapi.UserSegmentRule) (interface{}, error) {
	transformed := make([]interface{}, len(rules))

	for i, r := range rules {
		clauses, err := clausesToResourceData(r.Clauses)
		if err != nil {
			return nil, err
		}
		transformed[i] = map[string]interface{}{
			CLAUSES:   clauses,
			WEIGHT:    r.Weight,
			BUCKET_BY: r.BucketBy,
		}
	}

	return transformed, nil
}
