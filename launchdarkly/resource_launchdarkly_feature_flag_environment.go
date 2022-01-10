package launchdarkly

import (
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	ldapi "github.com/launchdarkly/api-client-go/v7"
)

func resourceFeatureFlagEnvironment() *schema.Resource {
	return &schema.Resource{
		Create: resourceFeatureFlagEnvironmentCreate,
		Read:   resourceFeatureFlagEnvironmentRead,
		Update: resourceFeatureFlagEnvironmentUpdate,
		Delete: resourceFeatureFlagEnvironmentDelete,

		Importer: &schema.ResourceImporter{
			State: resourceFeatureFlagEnvironmentImport,
		},
		Schema: baseFeatureFlagEnvironmentSchema(false),
	}
}

func validateFlagID(val interface{}, key string) (warns []string, errs []error) {
	v := val.(string)
	if strings.Count(v, "/") != 1 {
		return warns, append(errs, fmt.Errorf("%q must be in the format 'project_key/flag_key'. Got: %s", key, v))
	}
	for _, part := range strings.SplitN(v, "/", 2) {
		w, e := validateKey()(part, key)
		if len(e) > 0 {
			return w, e
		}
	}
	return warns, errs
}

func resourceFeatureFlagEnvironmentCreate(d *schema.ResourceData, metaRaw interface{}) error {
	client := metaRaw.(*Client)
	flagId := d.Get(FLAG_ID).(string)

	projectKey, flagKey, err := flagIdToKeys(flagId)
	if err != nil {
		return err
	}
	envKey := d.Get(ENV_KEY).(string)

	if exists, err := projectExists(projectKey, client); !exists {
		if err != nil {
			return err
		}
		return fmt.Errorf("cannot find project with key %q", projectKey)
	}

	if exists, err := environmentExists(projectKey, envKey, client); !exists {
		if err != nil {
			return err
		}
		return fmt.Errorf("failed to find environment with key %q", envKey)
	}

	patches := make([]ldapi.PatchOperation, 0)

	on := d.Get(ON)
	patches = append(patches, patchReplace(patchFlagEnvPath(d, "on"), on))

	// off_variation is required
	offVariation := d.Get(OFF_VARIATION)
	patches = append(patches, patchReplace(patchFlagEnvPath(d, "offVariation"), offVariation.(int)))

	trackEvents, ok := d.GetOk(TRACK_EVENTS)
	if ok {
		patches = append(patches, patchReplace(patchFlagEnvPath(d, "trackEvents"), trackEvents.(bool)))
	}

	_, ok = d.GetOk(RULES)
	if ok {
		rules, err := rulesFromResourceData(d)
		if err != nil {
			return err
		}
		patches = append(patches, patchReplace(patchFlagEnvPath(d, "rules"), rules))
	}

	_, ok = d.GetOk(PREREQUISITES)
	if ok {
		prerequisites := prerequisitesFromResourceData(d, PREREQUISITES)
		patches = append(patches, patchReplace(patchFlagEnvPath(d, "prerequisites"), prerequisites))
	}

	_, ok = d.GetOk(TARGETS)
	if ok {
		targets := targetsFromResourceData(d)
		patches = append(patches, patchReplace(patchFlagEnvPath(d, "targets"), targets))
	}

	// fallthrough is required
	fall, err := fallthroughFromResourceData(d)
	if err != nil {
		return err
	}
	patches = append(patches, patchReplace(patchFlagEnvPath(d, "fallthrough"), fall))

	if len(patches) > 0 {
		comment := "Terraform"
		patch := ldapi.PatchWithComment{
			Comment: &comment,
			Patch:   patches,
		}
		log.Printf("[DEBUG] %+v\n", patch)

		_, _, err = client.ld.FeatureFlagsApi.PatchFeatureFlag(client.ctx, projectKey, flagKey).PatchWithComment(patch).Execute()
		if err != nil {
			return fmt.Errorf("failed to update flag %q in project %q: %s", flagKey, projectKey, handleLdapiErr(err))
		}
	}

	d.SetId(projectKey + "/" + envKey + "/" + flagKey)
	return resourceFeatureFlagEnvironmentRead(d, metaRaw)
}

func resourceFeatureFlagEnvironmentRead(d *schema.ResourceData, metaRaw interface{}) error {
	return featureFlagEnvironmentRead(d, metaRaw, false)
}

func resourceFeatureFlagEnvironmentUpdate(d *schema.ResourceData, metaRaw interface{}) error {
	client := metaRaw.(*Client)
	flagId := d.Get(FLAG_ID).(string)
	projectKey, flagKey, err := flagIdToKeys(flagId)
	if err != nil {
		return err
	}
	envKey := d.Get(ENV_KEY).(string)

	if exists, err := projectExists(projectKey, client); !exists {
		if err != nil {
			return err
		}
		return fmt.Errorf("cannot find project with key %q", projectKey)
	}

	if exists, err := environmentExists(projectKey, envKey, client); !exists {
		if err != nil {
			return err
		}
		return fmt.Errorf("failed to find environment with key %q", envKey)
	}

	on := d.Get(ON)
	rules, err := rulesFromResourceData(d)
	if err != nil {
		return err
	}
	trackEvents := d.Get(TRACK_EVENTS).(bool)
	prerequisites := prerequisitesFromResourceData(d, PREREQUISITES)
	targets := targetsFromResourceData(d)

	fall, err := fallthroughFromResourceData(d)
	if err != nil {
		return err
	}
	offVariation := d.Get(OFF_VARIATION)

	comment := "Terraform"
	patch := ldapi.PatchWithComment{
		Comment: &comment,
		Patch: []ldapi.PatchOperation{
			patchReplace(patchFlagEnvPath(d, "on"), on),
			patchReplace(patchFlagEnvPath(d, "rules"), rules),
			patchReplace(patchFlagEnvPath(d, "trackEvents"), trackEvents),
			patchReplace(patchFlagEnvPath(d, "prerequisites"), prerequisites),
			patchReplace(patchFlagEnvPath(d, "targets"), targets),
			patchReplace(patchFlagEnvPath(d, "fallthrough"), fall),
			patchReplace(patchFlagEnvPath(d, "offVariation"), offVariation),
		}}

	log.Printf("[DEBUG] %+v\n", patch)
	_, _, err = client.ld.FeatureFlagsApi.PatchFeatureFlag(client.ctx, projectKey, flagKey).PatchWithComment(patch).Execute()
	if err != nil {
		return fmt.Errorf("failed to update flag %q in project %q, environment %q: %s", flagKey, projectKey, envKey, handleLdapiErr(err))
	}
	return resourceFeatureFlagEnvironmentRead(d, metaRaw)
}

func resourceFeatureFlagEnvironmentDelete(d *schema.ResourceData, metaRaw interface{}) error {
	client := metaRaw.(*Client)
	flagId := d.Get(FLAG_ID).(string)
	projectKey, flagKey, err := flagIdToKeys(flagId)
	if err != nil {
		return err
	}
	envKey := d.Get(ENV_KEY).(string)

	if exists, err := projectExists(projectKey, client); !exists {
		if err != nil {
			return err
		}
		return fmt.Errorf("cannot find project with key %q", projectKey)
	}

	if exists, err := environmentExists(projectKey, envKey, client); !exists {
		if err != nil {
			return err
		}
		return fmt.Errorf("failed to find environment with key %q", envKey)
	}

	flag, _, err := client.ld.FeatureFlagsApi.GetFeatureFlag(client.ctx, projectKey, flagKey).Execute()
	if err != nil {
		return fmt.Errorf("failed to update flag %q in project %q, environment %q: %s", flagKey, projectKey, envKey, handleLdapiErr(err))
	}

	// Set off variation to match default with how a rule is created
	offVariation := len(flag.Variations) - 1

	comment := "Terraform"
	patch := ldapi.PatchWithComment{
		Comment: &comment,
		Patch: []ldapi.PatchOperation{
			patchReplace(patchFlagEnvPath(d, "on"), false),
			patchReplace(patchFlagEnvPath(d, "rules"), []ldapi.Rule{}),
			patchReplace(patchFlagEnvPath(d, "trackEvents"), false),
			patchReplace(patchFlagEnvPath(d, "prerequisites"), []ldapi.Prerequisite{}),
			patchReplace(patchFlagEnvPath(d, "offVariation"), offVariation),
			patchReplace(patchFlagEnvPath(d, "targets"), []ldapi.Target{}),
			patchReplace(patchFlagEnvPath(d, "fallthough"), fallthroughModel{Variation: intPtr(0)}),
		}}
	log.Printf("[DEBUG] %+v\n", patch)

	_, _, err = client.ld.FeatureFlagsApi.PatchFeatureFlag(client.ctx, projectKey, flagKey).PatchWithComment(patch).Execute()
	if err != nil {
		return fmt.Errorf("failed to update flag %q in project %q, environment %q: %s", flagKey, projectKey, envKey, handleLdapiErr(err))
	}

	return nil
}

func resourceFeatureFlagEnvironmentImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	id := d.Id()

	if strings.Count(id, "/") != 2 {
		return nil, fmt.Errorf("found unexpected flag id format: %q expected format: 'project_key/env_key/flag_key'", id)
	}
	parts := strings.SplitN(id, "/", 3)
	projectKey, envKey, flagKey := parts[0], parts[1], parts[2]
	_ = d.Set(FLAG_ID, projectKey+"/"+flagKey)
	_ = d.Set(ENV_KEY, envKey)

	return []*schema.ResourceData{d}, nil
}
