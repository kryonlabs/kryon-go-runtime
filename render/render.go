// render/render.go
package render

import (
	"github.com/kryonlabs/kryon-go-runtime/krb"
	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	MaxRenderElements    = 1024
	InvalidResourceIndex = 0xFF
	BaseFontSize         = 18.0 // Base default font size, also used for WindowConfig.DefaultFontSize
)

type EventCallbackInfo struct {
	EventType   krb.EventType
	HandlerName string
}

type RenderElement struct {
	Header               krb.ElementHeader
	OriginalIndex        int
	Parent               *RenderElement
	Children             []*RenderElement
	BgColor              rl.Color
	FgColor              rl.Color
	BorderColor          rl.Color
	BorderWidths         [4]uint8 // Top, Right, Bottom, Left
	Padding              [4]uint8 // Top, Right, Bottom, Left
	ResolvedFontSize     float32  // Stores the actual font size after style, direct props, and inheritance. 0.0 means "unset".
	TextAlignment        uint8    // Corresponds to krb.LayoutAlignStart, Center, End
	Text                 string
	ResourceIndex        uint8 // Index into KRB Resource Table
	Texture              rl.Texture2D
	TextureLoaded        bool
	RenderX              float32
	RenderY              float32
	RenderW              float32
	RenderH              float32
	IntrinsicW           int // Can be used by layout for initial content size estimation
	IntrinsicH           int // Can be used by layout for initial content size estimation
	IsVisible            bool
	IsInteractive        bool // True if element type is Button, Input, or other interactive standard types
	IsActive             bool // General purpose active state flag, can be used by event handlers or custom logic
	ActiveStyleNameIndex uint8 // KRB String Table index for the name of an "active" style (optional)
	InactiveStyleNameIndex uint8 // KRB String Table index for the name of an "inactive/base" style (optional)
	EventHandlers        []EventCallbackInfo
	DocRef               *krb.Document // Reference to the parsed KRB document
	SourceElementName    string        // Debug name, usually from KRY id or component name
}

type WindowConfig struct {
	Width              int
	Height             int
	Title              string
	Resizable          bool
	ScaleFactor        float32  // Global UI scale factor
	DefaultBg          rl.Color // Window clear color
	DefaultFgColor     rl.Color // Root default foreground/text color for inheritance
	DefaultBorderColor rl.Color // Default for borders if width is set but color isn't
	DefaultFontSize    float32  // Root default font size for inheritance
	// DefaultFontFamily string // Future: if font families are supported
}

// Renderer defines the core interface that all Kryon rendering backends must implement.
type Renderer interface {
	// --- Initialization and Setup ---
	Init(config WindowConfig) error
	PrepareTree(doc *krb.Document, krbFilePath string) (roots []*RenderElement, config WindowConfig, err error)
	GetRenderTree() []*RenderElement // Returns all processed RenderElements (flat list)
	Cleanup()
	ShouldClose() bool

	// --- Frame Lifecycle (Refactored) ---
	BeginFrame()                                   // Prepares for drawing (e.g., BeginDrawing, ClearBackground)
	UpdateLayout(roots []*RenderElement)           // Calculates all element positions and sizes
	PollEventsAndProcessInteractions()             // Handles input, triggers callbacks based on fresh layout
	DrawFrame(roots []*RenderElement)              // Draws the UI using the computed layout
	EndFrame()                                     // Finalizes frame drawing (e.g., EndDrawing)

	// --- Event and Component Registration ---
	RegisterEventHandler(name string, handler func())
	RegisterCustomComponent(identifier string, handler CustomComponentHandler) error

	// --- Resource Management ---
	LoadAllTextures() error // Loads all image resources referenced in the KRB

	// --- Utilities for Custom Handlers or Advanced Operations ---
	// Allows a custom handler to trigger a layout pass for the children of a specific element.
	PerformLayoutChildrenOfElement(
		parent *RenderElement,
		parentClientOriginX, parentClientOriginY,
		availableClientWidth, availableClientHeight float32,
	)
	// Allows runtime changes to an element's style to be reflected visually.
	ReResolveElementVisuals(el *RenderElement)
}

// CustomDrawer interface allows a custom component to take over its own drawing logic.
type CustomDrawer interface {
	// Draw is called during the rendering phase for an element that is associated with this handler.
	// - el: The RenderElement to draw.
	// - scale: The current UI scale factor.
	// - rendererInstance: The Renderer instance, providing access to drawing primitives if needed.
	// Returns:
	// - skipStandardDraw: If true, the renderer's standard drawing logic for this element type will be skipped.
	// - err: Any error encountered during drawing.
	Draw(el *RenderElement, scale float32, rendererInstance Renderer) (skipStandardDraw bool, err error)
}

// CustomEventHandler interface allows a custom component to handle specific KRB events.
type CustomEventHandler interface {
	// HandleEvent is called when a standard KRB event (like Click) occurs on an element
	// associated with this handler, if that element also has a custom component identifier.
	// This allows custom components to react to standard events in a specialized way.
	// - el: The RenderElement that received the event.
	// - eventType: The type of KRB event that occurred (e.g., krb.EventTypeClick).
	// - rendererInstance: The Renderer instance.
	// Returns:
	// - handled: If true, indicates the event was fully handled by this custom handler,
	//            and standard KRB event callbacks for this event on this element might be skipped.
	// - err: Any error encountered.
    HandleEvent(el *RenderElement, eventType krb.EventType, rendererInstance Renderer) (handled bool, err error)
}

// CustomComponentHandler defines an interface for Go code that provides specialized behavior
// for elements identified as custom components (e.g., via the _componentName custom property).
type CustomComponentHandler interface {
	// HandleLayoutAdjustment allows custom components to make final modifications to their
	// own layout or the layout of their children or siblings after the standard layout pass.
	// - el: The RenderElement representing the custom component instance.
	// - doc: The parsed KRB document.
	// - rendererInstance: The Renderer instance, allowing calls to PerformLayoutChildrenOfElement if needed.
	// Returns: Any error encountered.
	HandleLayoutAdjustment(el *RenderElement, doc *krb.Document, rendererInstance Renderer) error

	// Note: A type implementing CustomComponentHandler can also optionally implement
	// CustomDrawer and/or CustomEventHandler if it needs to control drawing or standard events.
}

// DefaultWindowConfig provides sensible default values for the application window.
func DefaultWindowConfig() WindowConfig {
	return WindowConfig{
		Width:              800,
		Height:             600,
		Title:              "Kryon Application",
		Resizable:          true,
		ScaleFactor:        1.0,
		DefaultBg:          rl.NewColor(30, 30, 30, 255), // Dark Gray
		DefaultFgColor:     rl.RayWhite,                   // White text
		DefaultBorderColor: rl.Gray,                       // Neutral gray
		DefaultFontSize:    BaseFontSize,                  // Use the defined constant
	}
}