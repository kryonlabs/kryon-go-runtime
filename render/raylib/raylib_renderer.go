// render/raylib/raylib_renderer.go
package raylib

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings" // Keep for GetCustomPropertyValue and logging

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/waozixyz/kryon/impl/go/krb"
	"github.com/waozixyz/kryon/impl/go/render"
)

const baseFontSize = 18.0
const componentNameConventionKey = "_componentName"
const childrenSlotIDName = "children_host" // Convention for KRY-usage children slot

type RaylibRenderer struct {
	config          render.WindowConfig
	elements        []render.RenderElement // Stores all elements, including expanded ones
	roots           []*render.RenderElement
	loadedTextures  map[uint8]rl.Texture2D
	krbFileDir      string
	scaleFactor     float32
	docRef          *krb.Document
	eventHandlerMap map[string]func()
	customHandlers  map[string]render.CustomComponentHandler
}

func NewRaylibRenderer() *RaylibRenderer {
	return &RaylibRenderer{
		loadedTextures:  make(map[uint8]rl.Texture2D),
		scaleFactor:     1.0,
		eventHandlerMap: make(map[string]func()),
		customHandlers:  make(map[string]render.CustomComponentHandler),
	}
}

func (r *RaylibRenderer) Init(config render.WindowConfig) error {
	r.config = config
	r.scaleFactor = float32(math.Max(1.0, float64(config.ScaleFactor)))

	log.Printf("RaylibRenderer Init: Initializing window %dx%d. Title: '%s'. UI Scale: %.2f.",
		config.Width, config.Height, config.Title, r.scaleFactor)

	rl.InitWindow(int32(config.Width), int32(config.Height), config.Title)

	if config.Resizable {
		rl.SetWindowState(rl.FlagWindowResizable)
	} else {
		rl.ClearWindowState(rl.FlagWindowResizable)
		rl.SetWindowSize(config.Width, config.Height) // Enforce fixed size
	}

	rl.SetTargetFPS(60) // Or from config if specified

	if !rl.IsWindowReady() {
		return fmt.Errorf("RaylibRenderer Init: rl.InitWindow failed or window is not ready")
	}
	log.Println("RaylibRenderer Init: Raylib window is ready.")
	return nil
}

func (r *RaylibRenderer) Cleanup() {
	log.Println("RaylibRenderer Cleanup: Unloading textures...")
	unloadedCount := 0
	for resourceIdx, texture := range r.loadedTextures {
		if texture.ID > 0 { // Check if texture is valid before unloading
			rl.UnloadTexture(texture)
			unloadedCount++
		}
		delete(r.loadedTextures, resourceIdx) // Remove from map
	}
	log.Printf("RaylibRenderer Cleanup: Unloaded %d textures from cache.", unloadedCount)
	r.loadedTextures = make(map[uint8]rl.Texture2D) // Reinitialize map

	if rl.IsWindowReady() {
		log.Println("RaylibRenderer Cleanup: Closing Raylib window...")
		rl.CloseWindow()
	} else {
		log.Println("RaylibRenderer Cleanup: Raylib window was already closed or not initialized.")
	}
}

func (r *RaylibRenderer) ShouldClose() bool {
	return rl.IsWindowReady() && rl.WindowShouldClose()
}

func (r *RaylibRenderer) BeginFrame() {
	rl.BeginDrawing()
	rl.ClearBackground(r.config.DefaultBg)
}

func (r *RaylibRenderer) EndFrame() {
	rl.EndDrawing()
}

func (r *RaylibRenderer) GetRenderTree() []*render.RenderElement {
	if len(r.elements) == 0 {
		return nil
	}
	pointers := make([]*render.RenderElement, len(r.elements))
	for i := range r.elements {
		pointers[i] = &r.elements[i]
	}
	return pointers
}

// UpdateLayout calculates all element positions and sizes.
// This is called once per frame before event polling and drawing.
func (r *RaylibRenderer) UpdateLayout(roots []*render.RenderElement) {
	windowResized := rl.IsWindowResized()
	currentWidth := r.config.Width
	currentHeight := r.config.Height

	if windowResized && r.config.Resizable {
		newWidth := int(rl.GetScreenWidth())
		newHeight := int(rl.GetScreenHeight())
		if newWidth != currentWidth || newHeight != currentHeight {
			r.config.Width = newWidth
			r.config.Height = newHeight
			currentWidth = newWidth
			currentHeight = newHeight
			log.Printf("UpdateLayout: Window resized to %dx%d. Recalculating layout.", currentWidth, currentHeight)
		}
	} else if !r.config.Resizable {
		screenWidth := int(rl.GetScreenWidth())
		screenHeight := int(rl.GetScreenHeight())
		if currentWidth != screenWidth || currentHeight != screenHeight {
			rl.SetWindowSize(currentWidth, currentHeight)
		}
	}

	r.roots = roots // Store/update roots

	for _, root := range r.roots {
		if root != nil {
			r.PerformLayout(root, 0, 0, float32(currentWidth), float32(currentHeight))
		}
	}
	r.ApplyCustomComponentLayoutAdjustments()
}

func (r *RaylibRenderer) PerformLayoutChildrenOfElement(
	parent *render.RenderElement,
	parentClientOriginX, parentClientOriginY,
	availableClientWidth, availableClientHeight float32,
) {
	r.PerformLayoutChildren(parent, parentClientOriginX, parentClientOriginY, availableClientWidth, availableClientHeight)
}

func (r *RaylibRenderer) PollEventsAndProcessInteractions() {
	if !rl.IsWindowReady() {
		return
	}

	mousePos := rl.GetMousePosition()
	currentMouseCursor := rl.MouseCursorDefault // Start with default

	isMouseButtonClicked := rl.IsMouseButtonPressed(rl.MouseButtonLeft)
	clickHandledThisFrame := false            // Ensure only one click is processed per frame globally
	hoveredInteractiveElementThisFrame := false // Flag to ensure cursor is set by the topmost interactive element

	// Iterate in reverse order through all elements in the flat list.
	// This means elements added later (like expanded component children) are checked first.
	// This often (but not perfectly) approximates checking "topmost" elements first.
	for i := len(r.elements) - 1; i >= 0; i-- {
		el := &r.elements[i]

		isTabButton := strings.HasPrefix(el.SourceElementName, "tab_") // For specific logging

		if isTabButton {
			log.Printf("DEBUG PollEvents: Checking Tab Button '%s', Visible: %t, Interactive: %t, Bounds: (X:%.1f,Y:%.1f W:%.1f,H:%.1f), Mouse: (%.1f, %.1f)",
				el.SourceElementName, el.IsVisible, el.IsInteractive,
				el.RenderX, el.RenderY, el.RenderW, el.RenderH,
				mousePos.X, mousePos.Y)
		}

		if !el.IsVisible || el.RenderW <= 0 || el.RenderH <= 0 {
			if isTabButton { log.Printf("DEBUG PollEvents: Tab Button '%s' skipped (not visible or zero size).", el.SourceElementName); }
			continue
		}

		elementBounds := rl.NewRectangle(el.RenderX, el.RenderY, el.RenderW, el.RenderH)
		isMouseHoveringThisElement := rl.CheckCollisionPointRec(mousePos, elementBounds)

		if isTabButton {
			log.Printf("DEBUG PollEvents: Tab Button '%s', Hover Result: %t", el.SourceElementName, isMouseHoveringThisElement)
		}

		if isMouseHoveringThisElement {
			// An element (interactive or not) is under the mouse.
			// If it's interactive, it's our current best candidate for interaction.
			if el.IsInteractive {
				// Set the cursor to pointing hand only if we haven't already set it
				// for another interactive element "on top" of this one (which wouldn't
				// happen with this loop structure, but good for clarity).
				if !hoveredInteractiveElementThisFrame {
					currentMouseCursor = rl.MouseCursorPointingHand
					hoveredInteractiveElementThisFrame = true // Mark that an interactive element is handling hover
					if isTabButton { log.Printf("DEBUG PollEvents: Tab Button '%s' set cursor to PointingHand.", el.SourceElementName); }
				}

				// Process click ONLY for this topmost interactive element found so far
				if isMouseButtonClicked && !clickHandledThisFrame {
					if isTabButton { log.Printf("DEBUG PollEvents: Tab Button '%s' CLICK DETECTED.", el.SourceElementName); }
					
					eventWasProcessedByCustomHandler := false
					// Check for custom component event handling first
					componentID, isCustomInstance := GetCustomPropertyValue(el, componentNameConventionKey, r.docRef)
					if isCustomInstance && componentID != "" {
						if customHandler, handlerExists := r.customHandlers[componentID]; handlerExists {
							if eventInterface, implementsEvent := customHandler.(render.CustomEventHandler); implementsEvent {
								handled, err := eventInterface.HandleEvent(el, krb.EventTypeClick, r) // Pass renderer instance
								if err != nil {
									log.Printf("ERROR PollEvents: Custom click handler for '%s' [%s] returned error: %v",
										componentID, el.SourceElementName, err)
								}
								if handled {
									eventWasProcessedByCustomHandler = true
									clickHandledThisFrame = true
								}
							}
						}
					}

					// If not handled by custom, try standard KRB event handlers
					if !eventWasProcessedByCustomHandler && len(el.EventHandlers) > 0 {
						for _, eventInfo := range el.EventHandlers {
							if eventInfo.EventType == krb.EventTypeClick {
								goHandlerFunc, found := r.eventHandlerMap[eventInfo.HandlerName]
								if found {
									log.Printf("INFO: Click on '%s', executing handler '%s'", el.SourceElementName, eventInfo.HandlerName)
									goHandlerFunc()
									clickHandledThisFrame = true // Mark click as handled
								} else {
									log.Printf("Warn PollEvents: Standard KRB click handler named '%s' (for %s) is not registered.",
										eventInfo.HandlerName, el.SourceElementName)
								}
								break // Assuming one click action per element for this event type
							}
						}
					}
				}
				// Since we found an interactive element under the mouse, and we're iterating
				// from "latest added / potentially topmost child" to "earliest added / root",
				// this is the one that should get the interaction.
				// We can break the loop.
				break 
			}
			// If the element under the mouse is NOT interactive, we do nothing with it
			// regarding cursor or clicks. We continue the loop, because there might be
			// an interactive element "behind" this non-interactive one (in terms of r.elements order)
			// that is also under the mouse pointer (e.g. a small button on a large non-interactive panel).
			// The `hoveredInteractiveElementThisFrame` flag will ensure that if a *later* (in reverse iteration,
			// so visually "behind") interactive element is found, the cursor remains `PointingHand`.
		}
	}
	rl.SetMouseCursor(currentMouseCursor) // Set the cursor once at the end
}

func (r *RaylibRenderer) RegisterEventHandler(name string, handler func()) {
	if name == "" {
		log.Println("WARN RegisterEventHandler: Attempted to register handler with empty name.")
		return
	}
	if handler == nil {
		log.Printf("WARN RegisterEventHandler: Attempted to register nil handler for name '%s'.", name)
		return
	}
	if _, exists := r.eventHandlerMap[name]; exists {
		log.Printf("INFO RegisterEventHandler: Overwriting existing handler for event name '%s'", name)
	}
	r.eventHandlerMap[name] = handler
	log.Printf("Registered event handler for '%s'", name)
}

func (r *RaylibRenderer) RegisterCustomComponent(identifier string, handler render.CustomComponentHandler) error {
	if identifier == "" {
		return fmt.Errorf("RegisterCustomComponent: identifier cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("RegisterCustomComponent: handler cannot be nil for identifier '%s'", identifier)
	}
	if _, exists := r.customHandlers[identifier]; exists {
		log.Printf("INFO RegisterCustomComponent: Overwriting existing custom component handler for identifier '%s'", identifier)
	}
	r.customHandlers[identifier] = handler
	log.Printf("Registered custom component handler for '%s'", identifier)
	return nil
}

func (r *RaylibRenderer) LoadAllTextures() error {
	if r.docRef == nil {
		return fmt.Errorf("cannot load textures, KRB document reference is nil")
	}
	if !rl.IsWindowReady() {
		return fmt.Errorf("cannot load textures, Raylib window is not ready/initialized for GL operations")
	}

	log.Println("LoadAllTextures: Starting...")
	errCount := 0
	r.performTextureLoading(&errCount)
	log.Printf("LoadAllTextures: Complete. Encountered %d errors.", errCount)
	if errCount > 0 {
		return fmt.Errorf("encountered %d errors during texture loading", errCount)
	}
	return nil
}

func (r *RaylibRenderer) performTextureLoading(errorCounter *int) {
	if r.docRef == nil || r.elements == nil {
		log.Println("Error performTextureLoading: docRef or elements is nil.")
		if errorCounter != nil {
			*errorCounter++
		}
		return
	}

	for i := range r.elements {
		el := &r.elements[i]
		needsTexture := (el.Header.Type == krb.ElemTypeImage || el.Header.Type == krb.ElemTypeButton) &&
			el.ResourceIndex != render.InvalidResourceIndex
		if !needsTexture {
			continue
		}

		resIndex := el.ResourceIndex
		if int(resIndex) >= len(r.docRef.Resources) {
			log.Printf("Error performTextureLoading: Elem %s (GlobalIdx %d) ResourceIndex %d out of bounds for doc.Resources (len %d)",
				el.SourceElementName, el.OriginalIndex, resIndex, len(r.docRef.Resources))
			if errorCounter != nil {
				*errorCounter++
			}
			el.TextureLoaded = false
			continue
		}
		res := r.docRef.Resources[resIndex]

		if loadedTex, exists := r.loadedTextures[resIndex]; exists {
			el.Texture = loadedTex
			el.TextureLoaded = (loadedTex.ID > 0)
			if !el.TextureLoaded {
				log.Printf("Warn performTextureLoading: Cached texture for resource index %d was invalid. Re-attempting load.", resIndex)
				delete(r.loadedTextures, resIndex)
			} else {
				continue
			}
		}

		var texture rl.Texture2D
		loadedOk := false

		if res.Format == krb.ResFormatExternal {
			resourceName, nameOk := getStringValueByIdx(r.docRef, res.NameIndex)
			if !nameOk {
				log.Printf("Error performTextureLoading: Could not get resource name for external resource index: %d", res.NameIndex)
				if errorCounter != nil {
					*errorCounter++
				}
				el.TextureLoaded = false
				continue
			}
			fullPath := filepath.Join(r.krbFileDir, resourceName)
			if _, statErr := os.Stat(fullPath); os.IsNotExist(statErr) {
				log.Printf("Error performTextureLoading: External resource file not found: %s", fullPath)
				if errorCounter != nil {
					*errorCounter++
				}
				el.TextureLoaded = false
				continue
			}
			img := rl.LoadImage(fullPath)
			if img.Data == nil || img.Width == 0 || img.Height == 0 {
				log.Printf("Error performTextureLoading: Failed to load image data for external resource: %s", fullPath)
				if errorCounter != nil {
					*errorCounter++
				}
				rl.UnloadImage(img)
				el.TextureLoaded = false
				continue
			}
			texture = rl.LoadTextureFromImage(img)
			rl.UnloadImage(img)
			if texture.ID > 0 {
				loadedOk = true
			} else {
				log.Printf("Error performTextureLoading: Failed to create texture from image for %s", fullPath)
				if errorCounter != nil {
					*errorCounter++
				}
			}
		} else if res.Format == krb.ResFormatInline {
			if res.InlineData == nil || res.InlineDataSize == 0 {
				log.Printf("Error performTextureLoading: Inline resource data is nil or size 0 (name index: %d)", res.NameIndex)
				if errorCounter != nil {
					*errorCounter++
				}
				el.TextureLoaded = false
				continue
			}
			ext := ".png"
			img := rl.LoadImageFromMemory(ext, res.InlineData, int32(len(res.InlineData)))
			if img.Data == nil || img.Width == 0 || img.Height == 0 {
				log.Printf("Error performTextureLoading: Failed to load image data from inline resource (name index: %d, size: %d)", res.NameIndex, res.InlineDataSize)
				if errorCounter != nil {
					*errorCounter++
				}
				rl.UnloadImage(img)
				el.TextureLoaded = false
				continue
			}
			texture = rl.LoadTextureFromImage(img)
			rl.UnloadImage(img)
			if texture.ID > 0 {
				loadedOk = true
			} else {
				log.Printf("Error performTextureLoading: Failed to create texture from inline image data (name index %d)", res.NameIndex)
				if errorCounter != nil {
					*errorCounter++
				}
			}
		} else {
			log.Printf("Error performTextureLoading: Unknown resource format %d for resource (name index: %d)", res.Format, res.NameIndex)
			if errorCounter != nil {
				*errorCounter++
			}
		}

		if loadedOk {
			el.Texture = texture
			el.TextureLoaded = true
			r.loadedTextures[resIndex] = texture
		} else {
			el.TextureLoaded = false
		}
	}
}

// DrawFrame now only draws, using the layout computed by UpdateLayout.
// It fulfills the render.Renderer interface.
func (r *RaylibRenderer) DrawFrame(roots []*render.RenderElement) {
	r.roots = roots // Ensure r.roots is current if roots can change dynamically per frame
	for _, root := range r.roots {
		if root != nil {
			r.renderElementRecursiveWithCustomDraw(root, r.scaleFactor)
		}
	}
}

func (r *RaylibRenderer) ApplyCustomComponentLayoutAdjustments() {
	if r.docRef == nil || len(r.customHandlers) == 0 || len(r.elements) == 0 {
		return
	}
	for i := range r.elements {
		el := &r.elements[i]
		if el == nil {
			continue
		}
		componentIdentifier, found := GetCustomPropertyValue(el, componentNameConventionKey, r.docRef)
		if found && componentIdentifier != "" {
			handler, handlerFound := r.customHandlers[componentIdentifier]
			if handlerFound {
				err := handler.HandleLayoutAdjustment(el, r.docRef, r)
				if err != nil {
					log.Printf("ERROR ApplyCustomComponentLayoutAdjustments: Custom layout handler for '%s' [%s] failed: %v",
						componentIdentifier, el.SourceElementName, err)
				}
			}
		}
	}
}

func (r *RaylibRenderer) renderElementRecursiveWithCustomDraw(el *render.RenderElement, scale float32) {
	if el == nil || !el.IsVisible {
		return
	}

	skipStandardDraw := false
	var drawErr error
	componentIdentifier := ""
	foundName := false

	if r.docRef != nil {
		componentIdentifier, foundName = GetCustomPropertyValue(el, componentNameConventionKey, r.docRef)
	}

	if foundName && componentIdentifier != "" {
		if handler, foundHandler := r.customHandlers[componentIdentifier]; foundHandler {
			if drawer, ok := handler.(render.CustomDrawer); ok {
				skipStandardDraw, drawErr = drawer.Draw(el, scale, r)
				if drawErr != nil {
					log.Printf("ERROR renderElementRecursiveWithCustomDraw: Custom Draw handler for component '%s' [%s] failed: %v",
						componentIdentifier, el.SourceElementName, drawErr)
				}
			}
		}
	}

	if !skipStandardDraw {
		r.renderStandardElement(el, scale) // Changed name to avoid confusion
	} else {
		// If custom draw handles its own children, this loop might be skipped based on CustomDrawer's contract.
		for _, child := range el.Children {
			r.renderElementRecursiveWithCustomDraw(child, scale)
		}
	}
}

// renderStandardElement is the renamed renderElementRecursive for clarity
func (r *RaylibRenderer) renderStandardElement(el *render.RenderElement, scale float32) {
	if el == nil || !el.IsVisible { // Already checked by caller, but good for safety
		return
	}

	renderXf, renderYf, renderWf, renderHf := el.RenderX, el.RenderY, el.RenderW, el.RenderH

	if renderWf <= 0 || renderHf <= 0 {
		for _, child := range el.Children {
			r.renderElementRecursiveWithCustomDraw(child, scale)
		}
		return
	}

	renderX, renderY := int32(renderXf), int32(renderYf)
	renderW, renderH := int32(renderWf), int32(renderHf)

	effectiveBgColor := el.BgColor
	effectiveFgColor := el.FgColor
	borderColor := el.BorderColor

	// Simplified active/inactive style handling (assumes color changes mainly)
	if (el.Header.Type == krb.ElemTypeButton) && (el.ActiveStyleNameIndex != 0 || el.InactiveStyleNameIndex != 0) {
		// This `el.IsActive` flag would need to be set by some interaction logic
		// (e.g. if this button corresponds to the active tab)
		// For the tab bar example, this logic is in `updateTabStyles` which calls `ReResolveElementVisuals`.
		// `ReResolveElementVisuals` updates BgColor/FgColor based on the new style.
		// So, direct application here might be redundant if ReResolveVisuals is correctly used.
		// However, if IsActive is a more general flag, this can remain.
		// Let's assume ReResolveElementVisuals handles this correctly through style application.
	}

	if effectiveBgColor.A > 0 {
		rl.DrawRectangle(renderX, renderY, renderW, renderH, effectiveBgColor)
	}

	topBorder := scaledI32(el.BorderWidths[0], scale)
	rightBorder := scaledI32(el.BorderWidths[1], scale)
	bottomBorder := scaledI32(el.BorderWidths[2], scale)
	leftBorder := scaledI32(el.BorderWidths[3], scale)
	clampedTop, clampedBottom := clampOpposingBorders(int(topBorder), int(bottomBorder), int(renderH))
	clampedLeft, clampedRight := clampOpposingBorders(int(leftBorder), int(rightBorder), int(renderW))
	drawBorders(int(renderX), int(renderY), int(renderW), int(renderH),
		clampedTop, clampedRight, clampedBottom, clampedLeft, borderColor)

	paddingTop := scaledI32(el.Padding[0], scale)
	paddingRight := scaledI32(el.Padding[1], scale)
	paddingBottom := scaledI32(el.Padding[2], scale)
	paddingLeft := scaledI32(el.Padding[3], scale)

	contentX_f32 := renderXf + float32(clampedLeft) + float32(paddingLeft)
	contentY_f32 := renderYf + float32(clampedTop) + float32(paddingTop)
	contentWidth_f32 := renderWf - float32(clampedLeft) - float32(clampedRight) - float32(paddingLeft) - float32(paddingRight)
	contentHeight_f32 := renderHf - float32(clampedTop) - float32(clampedBottom) - float32(paddingTop) - float32(paddingBottom)

	contentX := int32(contentX_f32)
	contentY := int32(contentY_f32)
	contentWidth := maxI32(0, int32(contentWidth_f32))
	contentHeight := maxI32(0, int32(contentHeight_f32))

	if contentWidth > 0 && contentHeight > 0 {
		rl.BeginScissorMode(contentX, contentY, contentWidth, contentHeight)
		// Use el.ResolvedFontSize for text rendering
		scaledResolvedFontSize := MaxF(1.0, el.ResolvedFontSize*scale) // Use resolved font size
		r.drawContent(el, int(contentX), int(contentY), int(contentWidth), int(contentHeight), scale, effectiveFgColor, scaledResolvedFontSize)
		rl.EndScissorMode()
	}

	for _, child := range el.Children {
		r.renderElementRecursiveWithCustomDraw(child, scale)
	}
}

// drawContent now takes scaledResolvedFontSize
func (r *RaylibRenderer) drawContent(el *render.RenderElement, cx, cy, cw, ch int, scale float32, effectiveFgColor rl.Color, scaledResolvedFontSize float32) {
	if (el.Header.Type == krb.ElemTypeText || el.Header.Type == krb.ElemTypeButton) && el.Text != "" {
		fontSize := int32(scaledResolvedFontSize) // Use the passed, scaled, resolved font size
		if fontSize < 1 {
			fontSize = 1
		}

		textWidthMeasured := rl.MeasureText(el.Text, fontSize)
		textHeightMeasured := fontSize

		textDrawX := int32(cx)
		textDrawY := int32(cy + (ch-int(textHeightMeasured))/2)

		switch el.TextAlignment {
		case krb.LayoutAlignCenter:
			textDrawX = int32(cx + (cw-int(textWidthMeasured))/2)
		case krb.LayoutAlignEnd:
			textDrawX = int32(cx + cw - int(textWidthMeasured))
		}
		rl.DrawText(el.Text, textDrawX, textDrawY, fontSize, effectiveFgColor)
	}

	isImageElement := (el.Header.Type == krb.ElemTypeImage || el.Header.Type == krb.ElemTypeButton)
	if isImageElement && el.TextureLoaded && el.Texture.ID > 0 {
		texWidth := float32(el.Texture.Width)
		texHeight := float32(el.Texture.Height)
		sourceRec := rl.NewRectangle(0, 0, texWidth, texHeight)
		destRec := rl.NewRectangle(float32(cx), float32(cy), float32(cw), float32(ch))
		if destRec.Width > 0 && destRec.Height > 0 && sourceRec.Width > 0 && sourceRec.Height > 0 {
			rl.DrawTexturePro(el.Texture, sourceRec, destRec, rl.NewVector2(0, 0), 0.0, rl.White)
		}
	}
}

func drawBorders(x, y, w, h, top, right, bottom, left int, color rl.Color) {
	if color.A == 0 {
		return
	}
	if top > 0 {
		rl.DrawRectangle(int32(x), int32(y), int32(w), int32(top), color)
	}
	if bottom > 0 {
		rl.DrawRectangle(int32(x), int32(y+h-bottom), int32(w), int32(bottom), color)
	}
	sideY := y + top
	sideH := h - top - bottom
	if sideH > 0 {
		if left > 0 {
			rl.DrawRectangle(int32(x), int32(sideY), int32(left), int32(sideH), color)
		}
		if right > 0 {
			rl.DrawRectangle(int32(x+w-right), int32(sideY), int32(right), int32(sideH), color)
		}
	}
}

func (r *RaylibRenderer) GetKrbFileDir() string { return r.krbFileDir }

func clampOpposingBorders(borderA, borderB, totalSize int) (int, int) {
	if totalSize <= 0 {
		return 0, 0
	}
	if borderA < 0 {
		borderA = 0
	}
	if borderB < 0 {
		borderB = 0
	}
	if borderA+borderB > totalSize {
		sum := float32(borderA + borderB)
		borderA = int(float32(borderA) / sum * float32(totalSize))
		borderB = totalSize - borderA
	}
	return borderA, borderB
}
