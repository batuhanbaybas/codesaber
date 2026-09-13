package main

import (
	"embed"

	"log"
	"time"

	"codesaber/backend"
	"codesaber/backend/adapter"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

// setApplicationMenu installs the macOS application menu with the default
// app/edit/window roles plus File → Close Tab (⌘W), which forwards to the
// frontend as the 'codesaber:close-tab' event.
func setApplicationMenu(app *application.App) {
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)

	fileMenu := menu.AddSubmenu("File")
	closeTab := fileMenu.Add("Close Tab")
	closeTab.SetAccelerator("CmdOrCtrl+W")
	closeTab.OnClick(func(*application.Context) {
		app.Event.Emit("codesaber:close-tab")
	})
	// AI: Edit Selection routes ⌘K into the editor as an event; the frontend
	// decides whether a non-diff tab with a non-empty selection is active and
	// otherwise surfaces a status-bar hint.
	aiEdit := fileMenu.Add("AI: Edit Selection \u2318K")
	aiEdit.SetAccelerator("CmdOrCtrl+K")
	aiEdit.OnClick(func(*application.Context) {
		app.Event.Emit("codesaber:ai-edit")
	})
	// NOTE: no CloseWindow role here — the native performClose: item would
	// also claim ⌘W and race with Close Tab. The window itself stays closable
	// via the traffic lights / ⌘⇧W.

	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	app.Menu.Set(menu)
}


func init() {
	// Register a custom event whose associated data type is string.
	// This is not required, but the binding generator will pick up registered events
	// and provide a strongly typed JS/TS API for them.
	application.RegisterEvent[string]("time")
}

// main function serves as the application's entry point. It initializes the application, creates a window,
// and starts a goroutine that emits a time-based event every second. It subsequently runs the application and
// logs any error that might occur.
func main() {

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Bind' is a list of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	app := application.New(application.Options{
		Name:        "codesaber",
		Description: "codesaber IDE",
		Services: []application.Service{
			application.NewService(&GreetService{}),
			application.NewService(backend.New(adapter.NewBridge())),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// setApplicationMenu builds the macOS app menu. File → Close Tab is bound
	// to ⌘W: a native menu accelerator intercepts the chord before the
	// WKWebView sees it, so webview-level preventDefault alone cannot be
	// relied on to keep Cmd+W from reaching system handlers. The menu item
	// simply emits 'codesaber:close-tab'; the frontend decides whether the webview
	// handler or this fallback actually closes the tab (see Workspace.tsx).
	setApplicationMenu(app)

	// Startup shows the welcome window only; the workspace window is created
	// on demand from the backend RPC EnsureWorkspaceWindow (adapter/window.go).
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:    adapter.WelcomeWindowName,
		Title:   "codesaber",
		Width:   800,
		Height:  520,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 34,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(30, 31, 34),
		URL:              "/#welcome",
	})

	// Create a goroutine that emits an event containing the current time every second.
	// The frontend can listen to this event and update the UI accordingly.
	go func() {
		for {
			now := time.Now().Format(time.RFC1123)
			app.Event.Emit("time", now)
			time.Sleep(time.Second)
		}
	}()

	// Run the application. This blocks until the application has been exited.
	err := app.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}
