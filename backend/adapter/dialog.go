package adapter

import "github.com/wailsapp/wails/v3/pkg/application"

// PickFolder opens a native directory-picker dialog and returns the selected
// absolute path. It returns an empty string with a nil error when the user
// cancels.
func PickFolder(title string) (string, error) {
	app := application.Get()
	if app == nil {
		return "", nil
	}
	return app.Dialog.OpenFile().
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(false).
		TreatsFilePackagesAsDirectories(true).
		SetTitle(title).
		PromptForSingleSelection()
}
