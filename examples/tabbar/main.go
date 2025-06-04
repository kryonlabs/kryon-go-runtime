//go:generate kryc ../../../kryon-core/examples/tab_bar.kry ./tab_bar.krb

package main

import (
	"bytes"
	_ "embed"
	"log"

	"github.com/kryonlabs/kryon-go-runtime/go/krb"
	"github.com/kryonlabs/kryon-go-runtime/go/render"
	kraylib "github.com/kryonlabs/kryon-go-runtime/go/render/raylib"
)

//go:embed tab_bar.krb
var embeddedKrbData []byte

var (
	appRenderer          render.Renderer
	krbDocument          *krb.Document
	allElements          []*render.RenderElement // This is a slice of pointers
	tabItemStyleBaseID   uint8
	tabItemStyleActiveID uint8
	roots                []*render.RenderElement
)

func findElementByID(idName string) *render.RenderElement {
	if krbDocument == nil || allElements == nil {
		return nil
	}
	var targetStringIndex uint8 = 0xFF
	for i, s := range krbDocument.Strings {
		if s == idName {
			targetStringIndex = uint8(i)
			break
		}
	}
	if targetStringIndex == 0xFF {
		return nil
	}

	// Iterate through the slice of pointers
	for _, el := range allElements { // el is *render.RenderElement
		if el != nil && el.Header.ID == targetStringIndex {
			return el // Return the pointer directly
		}
	}
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

	if tabItemStyleBaseID == 0 || tabItemStyleActiveID == 0 {
		tabItemStyleBaseID = kraylib.FindStyleIDByName(krbDocument, "tab_item_style_base")
		tabItemStyleActiveID = kraylib.FindStyleIDByName(krbDocument, "tab_item_style_active_base")
		if tabItemStyleBaseID == 0 || tabItemStyleActiveID == 0 {
			log.Printf("ERROR: Could not find tab item style IDs by name. Base: %d, Active: %d.", tabItemStyleBaseID, tabItemStyleActiveID)
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
			if tabButton.Header.StyleID != newStyleID {
				log.Printf("INFO: Changing style for tab '%s' from StyleID %d to %d", tabButton.SourceElementName, tabButton.Header.StyleID, newStyleID)
				tabButton.Header.StyleID = newStyleID
				appRenderer.ReResolveElementVisuals(tabButton)
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
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

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
	krbDocument = doc
	log.Printf("INFO: Parsed KRB - Ver=%d.%d Elements=%d Styles=%d Strings=%d CompDefs=%d",
		doc.VersionMajor, doc.VersionMinor, doc.Header.ElementCount, doc.Header.StyleCount, doc.Header.StringCount, doc.Header.ComponentDefCount)

	if doc.Header.ElementCount == 0 {
		log.Fatal("ERROR: No elements found in KRB data.")
	}

	rendererImpl := kraylib.NewRaylibRenderer()
	appRenderer = rendererImpl

	err = rendererImpl.RegisterCustomComponent("TabBar", &kraylib.TabBarHandler{})
	if err != nil {
		log.Fatalf("Error registering TabBarHandler: %v", err)
	}

	rendererImpl.RegisterEventHandler("showHomePage", showHomePage)
	rendererImpl.RegisterEventHandler("showSearchPage", showSearchPage)
	rendererImpl.RegisterEventHandler("showProfilePage", showProfilePage)

	var windowConfig render.WindowConfig
	roots, windowConfig, err = rendererImpl.PrepareTree(doc, ".")
	if err != nil {
		log.Fatalf("ERROR: Failed to prepare render tree: %v", err)
	}
	if len(roots) == 0 && doc.Header.ElementCount > 0 {
		log.Fatal("ERROR: Render tree preparation resulted in no root elements.")
	}
	allElements = rendererImpl.GetRenderTree() // This returns []*render.RenderElement

	showHomePage()

	err = rendererImpl.Init(windowConfig)
	if err != nil {
		rendererImpl.Cleanup()
		log.Fatalf("ERROR: Failed to initialize renderer window: %v", err)
	}
	defer rendererImpl.Cleanup()

	if err := rendererImpl.LoadAllTextures(); err != nil {
		log.Printf("WARN: Error loading textures: %v", err)
	}

	log.Println("INFO: Entering main loop...")
	for !rendererImpl.ShouldClose() {
		rendererImpl.UpdateLayout(roots)
		rendererImpl.PollEventsAndProcessInteractions()

		rendererImpl.BeginFrame()
		rendererImpl.DrawFrame(roots)
		rendererImpl.EndFrame()
	}

	log.Println("INFO: Kryon TabBar example finished.")
}