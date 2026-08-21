package livefeed

// Info is the JSON representation of a camera's current state.
//
// This is a published contract: every /cameras endpoint in the project returns
// a JSON array of these, and the bundled web UI and any external client depend
// on these exact field names. The struct tags are therefore not cosmetic — do
// not rename a field without accepting that every consumer breaks.
type Info struct {
	// ID is the camera's numeric identifier as text, matching the {id} used in
	// the per-camera URL paths.
	ID string `json:"id"`
	// Index is the camera's position in the sorted listing, so a UI can lay
	// cameras out in a stable order without sorting them again.
	Index int `json:"index"`
	// Online reports whether a frame has arrived recently enough for the camera
	// to count as live. See offlineThreshold for what "recently" means.
	Online bool `json:"online"`
	// Rotation is the display rotation in degrees a viewer should apply. It is
	// always 0 from the restream proxies, which republish frames without
	// rotating them; only camera-web-api reports a real angle.
	Rotation int `json:"rotation"`
}
