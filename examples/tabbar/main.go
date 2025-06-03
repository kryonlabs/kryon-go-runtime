//go:generate kryc ../../../../examples/tab_bar.kry ./tab_bar.krb

package main

import (
	"bytes"
	_ "embed"
	"log"

	"github.com/waozixyz/kryon/impl/go/krb"    // KRB parser
	"github.com/waozixyz/kryon/impl/go/render" // Renderer interface
	// Import the specific implementation (Raylib)
	kraylib "github.com/waozixyz/kryon/impl/go/render/raylib" // aliased to kraylib
)

//go:embed tab_bar.krb
var embeddedKrbData []byte

var (
	// Global references for simplicity in event handlers.
	// For larger apps, consider a dedicated AppState struct or dependency injection.
	appRenderer render.Renderer
	krbDocument *krb.Document // Keep a reference to the parsed KRB document for string lookups
	allElements []*render.RenderElement
	// Style IDs for tab items - these should correspond to your KRY style definitions
	// It's better if the compiler/runtime can make these more robustly available,
	// but for this example, we'll assume they are known or can be looked up.
	// We'll try to look them up by name.
	tabItemStyleBaseID    uint8
	tabItemStyleActiveID  uint8
)

// findElementByID searches for an element by its KRY ID string.
// Note: KRY 'id' is compiled into KRB ElementHeader.ID (which is a string table index).
func findElementByID(idName string) *render.RenderElement {
	if krbDocument == nil || allElements == nil {
		return nil
	}
	var targetStringIndex uint8 = 0xFF // Invalid index
	for i, s := range krbDocument.Strings {
		if s == idName {
			targetStringIndex = uint8(i)
			break
		}
	}
	if targetStringIndex == 0xFF {
		log.Printf("WARN: ID '%s' not found in string table.", idName)
		return nil
	}

	for _, el := range allElements {
		if el.Header.ID == targetStringIndex { // Compare KRB ElementHeader.ID (string index)
			return el
		}
	}
	log.Printf("WARN: Element with ID '%s' (string index %d) not found in render tree.", idName, targetStringIndex)
	return nil
}

// updatePageVisibility sets the visibility of page containers.
func updatePageVisibility(activePageID string) {
	pageIDs := []string{"page_home", "page_search", "page_profile"}
	for _, pageID := range pageIDs {
		pageElement := findElementByID(pageID)
		if pageElement != nil {
			pageElement.IsVisible = (pageID == activePageID)
		}
	}
}

func updateTabStyles(activeTabID string) {
	tabIDs := []string{"tab_home", "tab_search", "tab_profile"}

	// Ensure style IDs are resolved once
	if tabItemStyleBaseID == 0 || tabItemStyleActiveID == 0 {
		// These names must match your style "name" in widgets/tab_bar.kry
		// Make sure FindStyleIDByName is exported or wrapped by an exported method on the renderer
		tabItemStyleBaseID = kraylib.FindStyleIDByName(krbDocument, "tab_item_style_base")
		tabItemStyleActiveID = kraylib.FindStyleIDByName(krbDocument, "tab_item_style_active_base")
		if tabItemStyleBaseID == 0 || tabItemStyleActiveID == 0 {
			log.Printf("ERROR: Could not find tab item style IDs by name. Base: %d, Active: %d. Check style names in KRY and KRB output.", tabItemStyleBaseID, tabItemStyleActiveID)
			return
		}
		log.Printf("INFO: Resolved Tab Style IDs - Base: %d, Active: %d", tabItemStyleBaseID, tabItemStyleActiveID)
	}

	for _, tabID := range tabIDs {
		tabButton := findElementByID(tabID)
		if tabButton != nil {
			newStyleID := tabItemStyleBaseID
			if tabID == activeTabID {
				newStyleID = tabItemStyleActiveID
			}

			// Only re-resolve if the style actually changes
			if tabButton.Header.StyleID != newStyleID {
				log.Printf("INFO: Changing style for tab '%s' from StyleID %d to %d", tabButton.SourceElementName, tabButton.Header.StyleID, newStyleID)
				tabButton.Header.StyleID = newStyleID

				// IMPORTANT: Call the renderer to re-resolve visuals for this element
				if rRenderer, ok := appRenderer.(*kraylib.RaylibRenderer); ok {
					rRenderer.ReResolveElementVisuals(tabButton)
				} else {
					log.Printf("ERROR: Could not cast appRenderer to RaylibRenderer to call ReResolveElementVisuals for '%s'", tabButton.SourceElementName)
				}
			}
		} else {
			log.Printf("WARN: Tab button with ID '%s' not found during style update.", tabID)
		}
	}
}


func showHomePage() {
	log.Println("ACTION: Show Home Page")
	updatePageVisibility("page_home")
	updateTabStyles("tab_home")
}

func showSearchPage() {
	log.Println("ACTION: Show Search Page")
	updatePageVisibility("page_search")
	updateTabStyles("tab_search")
}

func showProfilePage() {
	log.Println("ACTION: Show Profile Page")
	updatePageVisibility("page_profile")
	updateTabStyles("tab_profile")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile) // Optional: more detailed logging

	log.Println("INFO: Starting Kryon TabBar Example (Embedded KRB)")

	if len(embeddedKrbData) == 0 {
		log.Fatal("ERROR: Embedded KRB data is empty! Did you run 'go generate .' in this directory first?")
	}
	log.Printf("INFO: Using embedded KRB data (Size: %d bytes)", len(embeddedKrbData))

	krbReader := bytes.NewReader(embeddedKrbData)

	doc, err := krb.ReadDocument(krbReader)
	if err != nil {
		log.Fatalf("ERROR: Failed to parse embedded KRB: %v", err)
	}
	krbDocument = doc // Store for global access
	log.Printf("INFO: Parsed KRB - Ver=%d.%d Elements=%d Styles=%d Strings=%d CompDefs=%d",
		doc.VersionMajor, doc.VersionMinor, doc.Header.ElementCount, doc.Header.StyleCount, doc.Header.StringCount, doc.Header.ComponentDefCount)

	if doc.Header.ElementCount == 0 {
		log.Fatal("ERROR: No elements found in KRB data.")
	}

	renderer := kraylib.NewRaylibRenderer()
	appRenderer = renderer // Store global reference for handlers

	// --- Register Custom Component Handlers ---
	// The identifier "TabBar" must match the value of the `_componentName` custom property
	// that the Kryon compiler (`kryc`) adds to the placeholder element for TabBar instances.
	err = renderer.RegisterCustomComponent("TabBar", &kraylib.TabBarHandler{})
	if err != nil {
		log.Fatalf("Error registering TabBarHandler: %v", err)
	}

	// --- Register Event Handlers ---
	// These names MUST match the strings used in the KRY file's onClick properties.
	renderer.RegisterEventHandler("showHomePage", showHomePage)
	renderer.RegisterEventHandler("showSearchPage", showSearchPage)
	renderer.RegisterEventHandler("showProfilePage", showProfilePage)

	// --- Prepare Render Tree ---
	// "." indicates the current directory for resolving any *external* resources,
	// though for this embedded example, it's less critical for the KRB itself.
	roots, windowConfig, err := renderer.PrepareTree(doc, ".")
	if err != nil {
		log.Fatalf("ERROR: Failed to prepare render tree: %v", err)
	}
	if len(roots) == 0 && doc.Header.ElementCount > 0 {
		log.Fatal("ERROR: Render tree preparation resulted in no root elements.")
	}
	allElements = renderer.GetRenderTree() // Get the flat list of all RenderElements

	// Initial UI state
	showHomePage() // Show the home page and set initial tab style

	// --- Initialize Window ---
	err = renderer.Init(windowConfig)
	if err != nil {
		renderer.Cleanup()
		log.Fatalf("ERROR: Failed to initialize renderer window: %v", err)
	}
	defer renderer.Cleanup()

	log.Println("INFO: Entering main loop...")
	for !renderer.ShouldClose() {
		renderer.PollEvents()

		renderer.BeginFrame()
		renderer.RenderFrame(roots)
		renderer.EndFrame()
	}

	log.Println("INFO: Kryon TabBar example finished.")
}