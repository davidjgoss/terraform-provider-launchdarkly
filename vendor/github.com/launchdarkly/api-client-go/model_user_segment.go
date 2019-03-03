/*
 * LaunchDarkly REST API
 *
 * Build custom integrations with the LaunchDarkly REST API
 *
 * API version: 2.0.14
 * Contact: support@launchdarkly.com
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package ldapi

type UserSegment struct {
	// Unique identifier for the user segment.
	Key string `json:"key"`
	// Name of the user segment.
	Name string `json:"name"`
	// Description of the user segment.
	Description string `json:"description,omitempty"`
	// An array of tags for this user segment.
	Tags []string `json:"tags,omitempty"`
	// A unix epoch time in milliseconds specifying the creation time of this flag.
	CreationDate float32 `json:"creationDate"`
	// An array of user keys that are included in this segment.
	Included []string `json:"included,omitempty"`
	// An array of user keys that should not be included in this segment, unless they are also listed in \"included\".
	Excluded []string `json:"excluded,omitempty"`
	// An array of rules that can cause a user to be included in this segment.
	Rules []UserSegmentRule `json:"rules,omitempty"`
	Version int32 `json:"version,omitempty"`
	Links *Links `json:"_links,omitempty"`
}
