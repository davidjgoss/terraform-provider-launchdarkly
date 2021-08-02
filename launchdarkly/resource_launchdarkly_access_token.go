package launchdarkly

import (
	"fmt"
	"log"
	"net/http"

	"github.com/antihax/optional"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"

	ldapi "github.com/launchdarkly/api-client-go"
)

func resourceAccessToken() *schema.Resource {
	tokenPolicySchema := policyStatementsSchema(policyStatementSchemaOptions{
		conflictsWith: []string{ROLE, CUSTOM_ROLES, POLICY_STATEMENTS},
		description:   "An array of statements represented as config blocks with 3 attributes: effect, resources, actions. May be used in place of a built-in or custom role.",
	})

	deprecatedTokenPolicySchema := policyStatementsSchema(policyStatementSchemaOptions{
		description:   "An array of statements represented as config blocks with 3 attributes: effect, resources, actions. May be used in place of a built-in or custom role.",
		deprecated:    "'policy_statements' is deprecated in favor of 'inline_roles'. This field will be removed in the next major release of the LaunchDarkly provider",
		conflictsWith: []string{ROLE, CUSTOM_ROLES, INLINE_ROLES},
	})
	return &schema.Resource{
		Create: resourceAccessTokenCreate,
		Read:   resourceAccessTokenRead,
		Update: resourceAccessTokenUpdate,
		Delete: resourceAccessTokenDelete,
		Exists: resourceAccessTokenExists,

		Schema: map[string]*schema.Schema{
			NAME: {
				Type:        schema.TypeString,
				Description: "A human-friendly name for the access token",
				Optional:    true,
			},
			ROLE: {
				Type:          schema.TypeString,
				Description:   `The default built-in role for the token. Available options are "reader", "writer", and "admin"`,
				Optional:      true,
				ValidateFunc:  validation.StringInSlice([]string{"reader", "writer", "admin"}, false),
				ConflictsWith: []string{CUSTOM_ROLES, POLICY_STATEMENTS},
			},
			CUSTOM_ROLES: {
				Type:          schema.TypeSet,
				Description:   "A list of custom role IDs to use as access limits for the access token",
				Set:           schema.HashString,
				Elem:          &schema.Schema{Type: schema.TypeString},
				Optional:      true,
				ConflictsWith: []string{ROLE, POLICY_STATEMENTS},
			},
			POLICY_STATEMENTS: deprecatedTokenPolicySchema,
			INLINE_ROLES:      tokenPolicySchema,
			SERVICE_TOKEN: {
				Type:        schema.TypeBool,
				Description: "Whether the token is a service token",
				Optional:    true,
				ForceNew:    true,
				Default:     false,
			},
			DEFAULT_API_VERSION: {
				Type:         schema.TypeInt,
				Description:  "The default API version for this token",
				Optional:     true,
				ForceNew:     true,
				Computed:     true,
				ValidateFunc: validateAPIVersion,
			},
			TOKEN: {
				Type:        schema.TypeString,
				Description: "The access token used to authorize usage of the LaunchDarkly API",
				Computed:    true,
				Sensitive:   true,
			},
			EXPIRE: {
				Deprecated:  "'expire' is deprecated and will be removed in the next major release of the LaunchDarly provider",
				Type:        schema.TypeInt,
				Description: "Replace the computed token secret with a new value. The expired secret will no longer be able to authorize usage of the LaunchDarkly API. Should be an expiration time for the current token secret, expressed as a Unix epoch time in milliseconds. Setting this to a negative value will expire the existing token immediately. To reset the token value again, change 'expire' to a new value. Setting this field at resource creation time WILL NOT set an expiration time for the token.",
				Optional:    true,
			},
		},
	}
}

func validateAPIVersion(val interface{}, key string) (warns []string, errs []error) {
	v := val.(int)
	switch v {
	case 0, 20191212, 20160426:
		// do nothing
	default:
		errs = append(errs, fmt.Errorf("%q must be either `20191212` or `20160426`. Got: %v", key, v))
	}
	return warns, errs
}

func validateAccessTokenResource(d *schema.ResourceData) error {
	accessTokenRole := d.Get(ROLE).(string)
	customRolesRaw := d.Get(CUSTOM_ROLES).(*schema.Set).List()
	policyStatements, err := policyStatementsFromResourceData(d.Get(POLICY_STATEMENTS).([]interface{}))
	if err != nil {
		return err
	}

	inlineRoles, err := policyStatementsFromResourceData(d.Get(INLINE_ROLES).([]interface{}))
	if err != nil {
		return err
	}

	if accessTokenRole == "" && len(customRolesRaw) == 0 && len(policyStatements) == 0 && len(inlineRoles) == 0 {
		return fmt.Errorf("access_token must contain either 'role', 'custom_roles', 'policy_statements', or 'inline_roles'")
	}

	return nil
}

func resourceAccessTokenCreate(d *schema.ResourceData, metaRaw interface{}) error {
	err := validateAccessTokenResource(d)
	if err != nil {
		return err
	}

	client := metaRaw.(*Client)
	accessTokenName := d.Get(NAME).(string)
	accessTokenRole := d.Get(ROLE).(string)
	serviceToken := d.Get(SERVICE_TOKEN).(bool)
	defaultApiVersion := d.Get(DEFAULT_API_VERSION).(int)
	customRolesRaw := d.Get(CUSTOM_ROLES).(*schema.Set).List()
	inlineRoles, _ := policyStatementsFromResourceData(d.Get(POLICY_STATEMENTS).([]interface{}))
	if len(inlineRoles) == 0 {
		inlineRoles, _ = policyStatementsFromResourceData(d.Get(INLINE_ROLES).([]interface{}))
	}

	customRoles := make([]string, len(customRolesRaw))
	for i, cr := range customRolesRaw {
		customRoles[i] = cr.(string)
	}

	accessTokenBody := ldapi.TokenBody{
		Name:              accessTokenName,
		Role:              accessTokenRole,
		CustomRoleIds:     customRoles,
		InlineRole:        inlineRoles,
		ServiceToken:      serviceToken,
		DefaultApiVersion: int32(defaultApiVersion),
	}

	tokenRaw, _, err := handleRateLimit(func() (interface{}, *http.Response, error) {
		return client.ld.AccessTokensApi.PostToken(client.ctx, accessTokenBody)
	})
	token := tokenRaw.(ldapi.Token)
	if err != nil {
		return fmt.Errorf("failed to create access token with name %q: %s", accessTokenName, handleLdapiErr(err))
	}

	_ = d.Set(TOKEN, token.Token)
	d.SetId(token.Id)
	return resourceAccessTokenRead(d, metaRaw)
}

func resourceAccessTokenRead(d *schema.ResourceData, metaRaw interface{}) error {
	client := metaRaw.(*Client)
	accessTokenID := d.Id()

	accessTokenRaw, res, err := handleRateLimit(func() (interface{}, *http.Response, error) {
		return client.ld.AccessTokensApi.GetToken(client.ctx, accessTokenID)
	})
	accessToken := accessTokenRaw.(ldapi.Token)
	if isStatusNotFound(res) {
		log.Printf("[WARN] failed to find access token with id %q, removing from state", accessTokenID)
		d.SetId("")
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get access token with id %q: %s", accessTokenID, handleLdapiErr(err))
	}

	_ = d.Set(NAME, accessToken.Name)
	if accessToken.Role != "" {
		_ = d.Set(ROLE, accessToken.Role)
	}
	if len(accessToken.CustomRoleIds) > 0 {
		customRoleKeys, err := customRoleIDsToKeys(client, accessToken.CustomRoleIds)
		if err != nil {
			return err
		}
		_ = d.Set(CUSTOM_ROLES, customRoleKeys)
	}
	_ = d.Set(SERVICE_TOKEN, accessToken.ServiceToken)
	_ = d.Set(DEFAULT_API_VERSION, accessToken.DefaultApiVersion)

	policies := accessToken.InlineRole
	if len(policies) > 0 {
		policyStatements, _ := policyStatementsFromResourceData(d.Get(POLICY_STATEMENTS).([]interface{}))
		if len(policyStatements) > 0 {
			err = d.Set(POLICY_STATEMENTS, policyStatementsToResourceData(policies))
		} else {
			err = d.Set(INLINE_ROLES, policyStatementsToResourceData(policies))
		}
		if err != nil {
			return fmt.Errorf("could not set policy on access token with id %q: %v", accessTokenID, err)
		}
	}

	return nil
}

func resourceAccessTokenUpdate(d *schema.ResourceData, metaRaw interface{}) error {
	err := validateAccessTokenResource(d)
	if err != nil {
		return err
	}

	client := metaRaw.(*Client)
	accessTokenID := d.Id()
	accessTokenName := d.Get(NAME).(string)
	accessTokenRole := d.Get(ROLE).(string)
	customRolesRaw := d.Get(CUSTOM_ROLES).(*schema.Set).List()

	customRoleKeys := make([]string, len(customRolesRaw))
	for i, cr := range customRolesRaw {
		customRoleKeys[i] = cr.(string)
	}
	customRoleIds, err := customRoleKeysToIDs(client, customRoleKeys)
	if err != nil {
		return err
	}

	inlineRoles, _ := policyStatementsFromResourceData(d.Get(POLICY_STATEMENTS).([]interface{}))
	if len(inlineRoles) == 0 {
		inlineRoles, _ = policyStatementsFromResourceData(d.Get(INLINE_ROLES).([]interface{}))
	}
	iRoles := statementsToPolicies(inlineRoles)

	patch := []ldapi.PatchOperation{
		patchReplace("/name", &accessTokenName),
	}

	if d.HasChange(ROLE) {
		var op ldapi.PatchOperation
		if accessTokenRole == "" {
			op = patchRemove("/role")
		} else {
			op = patchReplace("/role", &accessTokenRole)
		}
		patch = append(patch, op)
	}
	if d.HasChange(CUSTOM_ROLES) {
		var op ldapi.PatchOperation
		if len(customRoleIds) == 0 {
			op = patchRemove("/customRoleIds")
		} else {
			op = patchReplace("/customRoleIds", &customRoleIds)
		}
		patch = append(patch, op)
	}
	if d.HasChange(POLICY_STATEMENTS) || d.HasChange(INLINE_ROLES) {
		var op ldapi.PatchOperation
		if len(iRoles) == 0 {
			op = patchRemove("/inlineRole")
		} else {
			op = patchReplace("/inlineRole", &iRoles)
		}
		patch = append(patch, op)
	}

	_, _, err = handleRateLimit(func() (interface{}, *http.Response, error) {
		return client.ld.AccessTokensApi.PatchToken(client.ctx, accessTokenID, patch)
	})
	if err != nil {
		return fmt.Errorf("failed to update access token with id %q: %s", accessTokenID, handleLdapiErr(err))
	}

	// Reset the access token if the expire field has been updated
	if d.HasChange(EXPIRE) {
		oldExpireRaw, newExpireRaw := d.GetChange(EXPIRE)
		oldExpire := oldExpireRaw.(int)
		newExpire := newExpireRaw.(int)
		opts := ldapi.AccessTokensApiResetTokenOpts{}
		if oldExpire != newExpire && newExpire != 0 {
			if newExpire > 0 {
				opts.Expiry = optional.NewInt64(int64(newExpire))
			}
			token, _, err := client.ld.AccessTokensApi.ResetToken(client.ctx, accessTokenID, &opts)
			if err != nil {
				return fmt.Errorf("failed to reset access token with id %q: %s", accessTokenID, handleLdapiErr(err))
			}
			_ = d.Set(EXPIRE, newExpire)
			_ = d.Set(TOKEN, token.Token)
		}
	}

	return resourceAccessTokenRead(d, metaRaw)
}

func resourceAccessTokenDelete(d *schema.ResourceData, metaRaw interface{}) error {
	client := metaRaw.(*Client)
	accessTokenID := d.Id()

	_, _, err := handleRateLimit(func() (interface{}, *http.Response, error) {
		res, err := client.ld.AccessTokensApi.DeleteToken(client.ctx, accessTokenID)
		return nil, res, err
	})
	if err != nil {
		return fmt.Errorf("failed to delete access token with id %q: %s", accessTokenID, handleLdapiErr(err))
	}

	return nil
}

func resourceAccessTokenExists(d *schema.ResourceData, metaRaw interface{}) (bool, error) {
	return accessTokenExists(d.Id(), metaRaw.(*Client))
}

func accessTokenExists(accessTokenID string, meta *Client) (bool, error) {
	_, res, err := meta.ld.AccessTokensApi.GetToken(meta.ctx, accessTokenID)
	if isStatusNotFound(res) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get access token with id %q: %s", accessTokenID, handleLdapiErr(err))
	}

	return true, nil
}