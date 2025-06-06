// render/raylib/renderer_processing.go
package raylib

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"path/filepath"
	//"strings" // Keep for PerformLayout logging condition

	rl "github.com/gen2brain/raylib-go/raylib" // For rl.Blank in expandComponent, default colors
	"github.com/kryonlabs/kryon-go-runtime/krb"
	"github.com/kryonlabs/kryon-go-runtime/render"
)

func (r *RaylibRenderer) PrepareTree(
	doc *krb.Document,
	krbFilePath string,
) ([]*render.RenderElement, render.WindowConfig, error) {

	if doc == nil {
		log.Println("PrepareTree: KRB document is nil.")
		return nil, r.config, fmt.Errorf("PrepareTree: KRB document is nil")
	}
	r.docRef = doc

	var err error
	r.krbFileDir, err = filepath.Abs(filepath.Dir(krbFilePath))
	if err != nil {
		r.krbFileDir = filepath.Dir(krbFilePath)
		log.Printf("WARN PrepareTree: Failed to get abs path for KRB file dir '%s': %v. Using relative: %s", krbFilePath, err, r.krbFileDir)
	}
	log.Printf("PrepareTree: Resource Base Directory set to: %s", r.krbFileDir)

	// --- 1. Initialize WindowConfig with application defaults ---
	windowConfig := render.DefaultWindowConfig() // Gets struct with hardcoded defaults

	// --- 2. Apply App Element's Style and Direct Properties to WindowConfig ---
	isAppElementPresent := (doc.Header.Flags&krb.FlagHasApp) != 0 &&
		doc.Header.ElementCount > 0 &&
		doc.Elements[0].Type == krb.ElemTypeApp

	if isAppElementPresent {
		appElementKrbHeader := &doc.Elements[0]
		// Apply style from App element to windowConfig
		if appStyle, styleFound := findStyle(doc, appElementKrbHeader.StyleID); styleFound {
			r.applyStylePropertiesToWindowConfig(appStyle.Properties, doc, &windowConfig)
		} else if appElementKrbHeader.StyleID != 0 {
			log.Printf("Warn PrepareTree: App element has StyleID %d, but style was not found.", appElementKrbHeader.StyleID)
		}
		// Apply direct properties from App element to windowConfig
		if len(doc.Properties) > 0 && len(doc.Properties[0]) > 0 {
			r.applyDirectPropertiesToWindowConfig(doc.Properties[0], doc, &windowConfig)
		}
	} else {
		log.Println("PrepareTree: No App element found in KRB. Using default window configuration.")
	}
	// Finalize scale factor and store config in renderer
	r.scaleFactor = float32(math.Max(1.0, float64(windowConfig.ScaleFactor)))
	r.config = windowConfig
	log.Printf("PrepareTree: Final Window Config: W:%d, H:%d, Title:'%s', Scale:%.2f, Resizable:%t, DefBG:%v, DefFG:%v, DefBorder:%v",
		r.config.Width, r.config.Height, r.config.Title, r.scaleFactor, r.config.Resizable, r.config.DefaultBg, r.config.DefaultFgColor, r.config.DefaultBorderColor)

	// --- 3. Process KRB Elements into RenderElements ---
	initialElementCount := int(doc.Header.ElementCount)
	if initialElementCount == 0 {
		log.Println("PrepareTree: No elements in KRB document.")
		r.elements = nil
		r.roots = nil
		return nil, r.config, nil
	}
	r.elements = make([]render.RenderElement, initialElementCount, initialElementCount*2)

	// Initial properties that are not typically styled or inherited directly in the first pass
	defaultTextAlignment := uint8(krb.LayoutAlignStart)
	defaultIsVisible := true

	for i := 0; i < initialElementCount; i++ {
		renderEl := &r.elements[i]
		krbElHeader := doc.Elements[i]

		// Basic Initialization
		renderEl.Header = krbElHeader
		renderEl.OriginalIndex = i
		renderEl.DocRef = doc
		renderEl.BgColor = rl.Blank     // Default: transparent
		renderEl.FgColor = rl.Blank     // Default: "unset", to be filled by style, direct, or inheritance
		renderEl.BorderColor = rl.Blank // Default: "unset"
		renderEl.BorderWidths = [4]uint8{0, 0, 0, 0}
		renderEl.Padding = [4]uint8{0, 0, 0, 0}
		renderEl.TextAlignment = defaultTextAlignment // Base default, can be overridden
		renderEl.IsVisible = defaultIsVisible         // Base default, can be overridden
		renderEl.IsInteractive = (krbElHeader.Type == krb.ElemTypeButton || krbElHeader.Type == krb.ElemTypeInput)
		renderEl.ResourceIndex = render.InvalidResourceIndex

		// Source Element Name for Debugging
		elementIDString, _ := getStringValueByIdx(doc, renderEl.Header.ID)
		var componentName string
		if doc.CustomProperties != nil && i < len(doc.CustomProperties) {
			componentName, _ = GetCustomPropertyValue(renderEl, componentNameConventionKey, doc)
		}
		if componentName != "" {
			renderEl.SourceElementName = componentName
		} else if elementIDString != "" {
			renderEl.SourceElementName = elementIDString
		} else {
			renderEl.SourceElementName = fmt.Sprintf("Type0x%X_Idx%d", renderEl.Header.Type, renderEl.OriginalIndex)
		}

		// Styling Resolution Order (as per spec section 5)
		// 5.1. Basic Init (done above)
		// 5.2. Style Application
		elementStyle, styleFound := findStyle(doc, krbElHeader.StyleID)
		if styleFound {
			r.applyStylePropertiesToElement(elementStyle.Properties, doc, renderEl)
		} else if krbElHeader.StyleID != 0 && !(i == 0 && isAppElementPresent) {
			log.Printf("Warn PrepareTree: Element %s (Idx %d) has StyleID %d, but style was not found.",
				renderEl.SourceElementName, i, krbElHeader.StyleID)
		}

		// 5.3. Direct Property Application (overrides style)
		if len(doc.Properties) > i && len(doc.Properties[i]) > 0 {
			if i == 0 && isAppElementPresent { // App element has some visual props for its RenderElement
				r.applyDirectVisualPropertiesToAppElement(doc.Properties[0], doc, renderEl)
			} else {
				r.applyDirectPropertiesToElement(doc.Properties[i], doc, renderEl)
			}
		}

		// Resolve text and image source (might use values from style or direct props)
		r.resolveElementTextAndImage(doc, renderEl, elementStyle, styleFound)

		// 5.4. Contextual Default Resolution (e.g., borders)
		r.applyContextualDefaults(renderEl)

		// Event handlers (not styling, but part of element setup)
		resolveEventHandlers(doc, renderEl) // This can stay here or move to utils
	}

	// --- 4. Link Original KRB Children & Expand Components ---
	kryUsageChildrenMap := make(map[int][]*render.RenderElement)
	if err_link := r.linkOriginalKrbChildren(initialElementCount, kryUsageChildrenMap); err_link != nil {
		return nil, r.config, fmt.Errorf("PrepareTree: failed during initial child linking: %w", err_link)
	}

	nextMasterIndex := initialElementCount
	for i := 0; i < initialElementCount; i++ {
		instanceElement := &r.elements[i]
		componentName, _ := GetCustomPropertyValue(instanceElement, componentNameConventionKey, doc)
		if componentName != "" {
			compDef := r.findComponentDefinition(componentName)
			if compDef != nil {
				instanceKryChildren := kryUsageChildrenMap[instanceElement.OriginalIndex]
				err_expand := r.expandComponent(instanceElement, compDef, &r.elements, &nextMasterIndex, instanceKryChildren)
				if err_expand != nil {
					log.Printf("ERROR PrepareTree: Failed to expand component '%s' for instance '%s': %v", componentName, instanceElement.SourceElementName, err_expand)
				}
			} else {
				log.Printf("Warn PrepareTree: Component definition for '%s' (instance '%s') not found.", componentName, instanceElement.SourceElementName)
			}
		}
	}

	// Finalize tree structure (Parent pointers and finding roots) *after* expansion
	r.roots = nil
	if err_build := r.finalizeTreeStructureAndRoots(); err_build != nil {
		return nil, r.config, fmt.Errorf("failed to finalize full element tree: %w", err_build)
	}

	// --- 5. Resolve Property Inheritance ---
	// This must happen *after* the full tree is linked and components are expanded,
	// so parent properties are fully resolved before children try to inherit.
	r.resolvePropertyInheritance()

	// --- Done with Tree Preparation ---
	log.Printf("PrepareTree: Tree built. Roots: %d. Total elements (incl. expanded): %d.", len(r.roots), len(r.elements))
	for rootIdx, rootNode := range r.roots {
		logElementTree(rootNode, 0, fmt.Sprintf("Root[%d]", rootIdx))
	}

	return r.roots, r.config, nil
}

func (r *RaylibRenderer) linkOriginalKrbChildren(
	initialElementCount int,
	kryUsageChildrenMap map[int][]*render.RenderElement,
) error {

	if r.docRef == nil || r.docRef.ElementStartOffsets == nil {
		return fmt.Errorf("linkOriginalKrbChildren: docRef or ElementStartOffsets is nil")
	}

	// Map KRB element file offsets to their index in the initial r.elements slice
	offsetToInitialElementIndex := make(map[uint32]int)

	for i := 0; i < initialElementCount && i < len(r.docRef.ElementStartOffsets); i++ {
		offsetToInitialElementIndex[r.docRef.ElementStartOffsets[i]] = i
	}

	for i := 0; i < initialElementCount; i++ {
		currentEl := &r.elements[i]
		originalKrbHeader := &r.docRef.Elements[i] // This is element from doc.Elements
		componentName, _ := GetCustomPropertyValue(currentEl, componentNameConventionKey, r.docRef)
		isPlaceholder := (componentName != "") // Is this element an instance of a component?

		if originalKrbHeader.ChildCount > 0 {
			// Ensure ChildRefs exist for this element in the KRB document

			if i >= len(r.docRef.ChildRefs) || r.docRef.ChildRefs[i] == nil {
				log.Printf(
					"Warn linkOriginalKrbChildren: Elem %s (OrigIdx %d) has KRB ChildCount %d but no ChildRefs in doc.",
					currentEl.SourceElementName, i, originalKrbHeader.ChildCount,
				)
				continue // Skip if child references are missing
			}

			krbChildRefs := r.docRef.ChildRefs[i]
			actualChildren := make([]*render.RenderElement, 0, len(krbChildRefs))

			parentStartOffset := uint32(0)

			if i < len(r.docRef.ElementStartOffsets) {
				parentStartOffset = r.docRef.ElementStartOffsets[i]
			} else {
				log.Printf(
					"Error linkOriginalKrbChildren: Elem %s (OrigIdx %d) missing from ElementStartOffsets.",
					currentEl.SourceElementName, i,
				)
				continue // Should not happen if initialElementCount is consistent
			}

			for _, childRef := range krbChildRefs {
				// ChildOffset in KRB is relative to the parent element's start in the file
				childAbsoluteFileOffset := parentStartOffset + uint32(childRef.ChildOffset)
				childIndexInInitialElements, found := offsetToInitialElementIndex[childAbsoluteFileOffset]

				if !found {
					log.Printf(
						"Error linkOriginalKrbChildren: Elem %s (OrigIdx %d) ChildRef offset %d (abs %d) does not map to known initial element.",
						currentEl.SourceElementName, i, childRef.ChildOffset, childAbsoluteFileOffset,
					)
					continue
				}
				childEl := &r.elements[childIndexInInitialElements]
				actualChildren = append(actualChildren, childEl)
			}

			if isPlaceholder {
				// For component instances, store these children temporarily. They will be slotted later.
				kryUsageChildrenMap[i] = actualChildren
			} else {
				// For regular elements, directly link children and set parent pointers
				currentEl.Children = actualChildren
				for _, child := range actualChildren {
					child.Parent = currentEl
				}
			}
		}
	}
	return nil
}

func (r *RaylibRenderer) finalizeTreeStructureAndRoots() error {

	if len(r.elements) == 0 {
		r.roots = nil
		return nil
	}
	r.roots = nil // Clear any existing roots

	for i := range r.elements {
		el := &r.elements[i] // Get pointer to the element

		if el.Parent == nil {
			r.roots = append(r.roots, el)
		}
	}

	if len(r.roots) == 0 && len(r.elements) > 0 {
		log.Printf(
			"Warn finalizeTreeStructureAndRoots: No root elements identified, but %d elements exist. This might indicate a problem in parent linking or an unusual KRB structure.",
			len(r.elements),
		)
	}
	return nil
}

func (r *RaylibRenderer) findComponentDefinition(name string) *krb.KrbComponentDefinition {

	if r.docRef == nil || len(r.docRef.ComponentDefinitions) == 0 || len(r.docRef.Strings) == 0 {
		return nil
	}

	for i := range r.docRef.ComponentDefinitions {
		compDef := &r.docRef.ComponentDefinitions[i]

		if int(compDef.NameIndex) < len(r.docRef.Strings) && r.docRef.Strings[compDef.NameIndex] == name {
			return compDef
		}
	}
	return nil
}

func findStyleIDByName(doc *krb.Document, name string) uint8 {
	if doc == nil || name == "" {
		return 0
	}
	for i := range doc.Styles { // Iterate by index to get pointer
		style := &doc.Styles[i]
		if styleName, ok := getStringValueByIdx(doc, style.NameIndex); ok && styleName == name {
			return style.ID // KRB Style.ID is 1-based
		}
	}
	return 0
}

func (r *RaylibRenderer) expandComponent(
	instanceElement *render.RenderElement, // The placeholder element being replaced
	compDef *krb.KrbComponentDefinition, // The definition of the component to expand
	allElements *[]render.RenderElement, // Pointer to the global slice of all elements
	nextMasterIndex *int, // Pointer to the next available global index for new elements
	kryUsageChildren []*render.RenderElement, // Children passed to the component instance in KRY
) error {
	doc := r.docRef
	compDefNameStr := getStringValueByIdxFallback(doc, compDef.NameIndex, "UnnamedComponentDef")

	log.Printf("Debug expandComponent: Expanding instance '%s' (OrigIdx %d, NameIdx %d) with definition '%s'. Initial nextMasterIndex: %d",
		instanceElement.SourceElementName, instanceElement.OriginalIndex, instanceElement.Header.ID, compDefNameStr, *nextMasterIndex)

	if compDef.RootElementTemplateData == nil || len(compDef.RootElementTemplateData) == 0 {
		log.Printf("Warn expandComponent: CompDef '%s' for instance '%s' has no RootElementTemplateData.", compDefNameStr, instanceElement.SourceElementName)
		// Slot KRY children directly to instance if no template and instance is a container
		if len(kryUsageChildren) > 0 && instanceElement.Header.Type == krb.ElemTypeContainer {
			log.Printf("Debug expandComponent: No template for '%s', appending %d KRY-usage children directly to instance.", instanceElement.SourceElementName, len(kryUsageChildren))
			if instanceElement.Children == nil {
				instanceElement.Children = make([]*render.RenderElement, 0, len(kryUsageChildren))
			}
			instanceElement.Children = append(instanceElement.Children, kryUsageChildren...)
			for _, kryChild := range kryUsageChildren {
				kryChild.Parent = instanceElement
			}
		}
		return nil
	}

	templateReader := bytes.NewReader(compDef.RootElementTemplateData)

	// Stores elements created *from this specific template expansion pass*.
	// Key: offset within template data stream, Value: global index in allElements
	localTemplateOffsetToGlobalIndex := make(map[uint32]int)

	// Stores child linking information for elements *within this template*.
	// parentGlobalIndex refers to an element created in this pass from this template.
	var localTemplateChildInfos []struct {
		parentGlobalIndex            int
		childRefs                    []krb.ChildRef
		parentHeaderOffsetInTemplate uint32 // Offset of parent's header in template data stream
	}

	var currentTemplateRootGlobalIndex = -1 // Global index of the root element of THIS template expansion
	templateDataStreamOffset := uint32(0)
	elementsCreatedInThisExpansionPass := 0

	// --- PASS 1: Create RenderElements from this template's data. Handle nested expansions. ---
	for templateReader.Len() > 0 {
		currentElementHeaderOffsetInTemplate := templateDataStreamOffset
		headerBuf := make([]byte, krb.ElementHeaderSize)
		n, err := templateReader.Read(headerBuf)
		if err == io.EOF {
			break
		}
		if err != nil || n < krb.ElementHeaderSize {
			return fmt.Errorf("expandComponent '%s' for instance '%s': failed to read template element header: %w (read %d bytes)", compDefNameStr, instanceElement.SourceElementName, err, n)
		}
		templateDataStreamOffset += uint32(n)
		elementsCreatedInThisExpansionPass++

		templateKrbHeader := krb.ElementHeader{
			Type:            krb.ElementType(headerBuf[0]),
			ID:              headerBuf[1],
			PosX:            krb.ReadU16LE(headerBuf[2:4]),
			PosY:            krb.ReadU16LE(headerBuf[4:6]),
			Width:           krb.ReadU16LE(headerBuf[6:8]),
			Height:          krb.ReadU16LE(headerBuf[8:10]),
			Layout:          headerBuf[10],
			StyleID:         headerBuf[11],
			PropertyCount:   headerBuf[12],
			ChildCount:      headerBuf[13],
			EventCount:      headerBuf[14],
			AnimationCount:  headerBuf[15],
			CustomPropCount: headerBuf[16],
		}

		newElGlobalIndex := *nextMasterIndex
		(*nextMasterIndex)++

		// Grow allElements slice if needed
		if newElGlobalIndex >= cap(*allElements) {
			newCap := cap(*allElements)*2 + 20 // Or other growth strategy
			tempSlice := make([]render.RenderElement, len(*allElements), newCap)
			copy(tempSlice, *allElements)
			*allElements = tempSlice
		}
		// Ensure slice has enough length
		if newElGlobalIndex >= len(*allElements) {
			*allElements = (*allElements)[:newElGlobalIndex+1]
		}

		newEl := &(*allElements)[newElGlobalIndex] // This is the RenderElement created from the template
		newEl.OriginalIndex = newElGlobalIndex
		newEl.Header = templateKrbHeader
		newEl.DocRef = doc
		newEl.BgColor = rl.Blank
		newEl.FgColor = rl.Blank
		newEl.BorderColor = rl.Blank
		newEl.BorderWidths = [4]uint8{}
		newEl.Padding = [4]uint8{}
		newEl.TextAlignment = UnsetTextAlignmentSentinel // Use sentinel for inheritance check
		newEl.IsVisible = true
		newEl.ResourceIndex = render.InvalidResourceIndex
		newEl.IsInteractive = (templateKrbHeader.Type == krb.ElemTypeButton || templateKrbHeader.Type == krb.ElemTypeInput)

		localTemplateOffsetToGlobalIndex[currentElementHeaderOffsetInTemplate] = newElGlobalIndex

		templateElIdStr, _ := getStringValueByIdx(doc, templateKrbHeader.ID)
		newEl.SourceElementName = templateElIdStr
		if newEl.SourceElementName == "" {
			newEl.SourceElementName = fmt.Sprintf("TplElem_Type0x%X_Idx%d_In_%s", templateKrbHeader.Type, newEl.OriginalIndex, compDefNameStr)
		}

		var templateDirectProps []krb.Property
		if templateKrbHeader.PropertyCount > 0 {
			templateDirectProps = make([]krb.Property, templateKrbHeader.PropertyCount)
			propHeaderBuf := make([]byte, 3) // ID(1), ValueType(1), Size(1)
			for j := uint8(0); j < templateKrbHeader.PropertyCount; j++ {
				nProp, errProp := templateReader.Read(propHeaderBuf)
				if errProp != nil || nProp < 3 {
					return fmt.Errorf("expandComp '%s': read prop header for '%s': %w", compDefNameStr, newEl.SourceElementName, errProp)
				}
				templateDataStreamOffset += uint32(nProp)
				prop := &templateDirectProps[j]
				prop.ID = krb.PropertyID(propHeaderBuf[0])
				prop.ValueType = krb.ValueType(propHeaderBuf[1])
				prop.Size = propHeaderBuf[2]
				if prop.Size > 0 {
					prop.Value = make([]byte, prop.Size)
					nVal, errVal := templateReader.Read(prop.Value)
					if errVal != nil || nVal < int(prop.Size) {
						return fmt.Errorf("expandComp '%s': read prop value for '%s': %w", compDefNameStr, newEl.SourceElementName, errVal)
					}
					templateDataStreamOffset += uint32(nVal)
				}
			}
		}

		var nestedComponentNameForThisNewEl string
		if templateKrbHeader.CustomPropCount > 0 {
			customPropHeaderBuf := make([]byte, 3) // KeyIndex(1), ValueType(1), Size(1)
			for j := uint8(0); j < templateKrbHeader.CustomPropCount; j++ {
				nCustomProp, errCustomProp := templateReader.Read(customPropHeaderBuf)
				if errCustomProp != nil || nCustomProp < 3 {
					return fmt.Errorf("expandComp '%s': read custom prop header for '%s': %w", compDefNameStr, newEl.SourceElementName, errCustomProp)
				}
				templateDataStreamOffset += uint32(nCustomProp)

				cpropKeyIndex := customPropHeaderBuf[0]
				cpropValueType := krb.ValueType(customPropHeaderBuf[1])
				cpropSize := customPropHeaderBuf[2]
				var cpropValue []byte
				if cpropSize > 0 {
					cpropValue = make([]byte, cpropSize)
					nVal, errVal := templateReader.Read(cpropValue)
					if errVal != nil || nVal < int(cpropSize) {
						return fmt.Errorf("expandComp '%s': read custom prop value for '%s': %w", compDefNameStr, newEl.SourceElementName, errVal)
					}
					templateDataStreamOffset += uint32(nVal)
				}

				keyName, keyOk := getStringValueByIdx(doc, cpropKeyIndex)
				if keyOk && keyName == componentNameConventionKey {
					if (cpropValueType == krb.ValTypeString || cpropValueType == krb.ValTypeResource) && cpropSize == 1 && len(cpropValue) == 1 {
						valueIndex := cpropValue[0]
						if strVal, strOk := getStringValueByIdx(doc, valueIndex); strOk {
							nestedComponentNameForThisNewEl = strVal
						}
					}
				}
			}
		}

		// Apply styling and properties based on whether it's template root or child
		if currentTemplateRootGlobalIndex == -1 { // This is the first element from template data stream
			currentTemplateRootGlobalIndex = newElGlobalIndex
			newEl.Parent = instanceElement // Its parent is the instance element being expanded
			log.Printf("Debug expandComponent [%s for %s]: Template root '%s' (GlobalIdx %d) created. Parent set to instance '%s' (GlobalIdx %d).",
				compDefNameStr, instanceElement.SourceElementName, newEl.SourceElementName, newEl.OriginalIndex, instanceElement.SourceElementName, instanceElement.OriginalIndex)

			// Apply instance's (placeholder) properties TO this template root
			newEl.Header.ID = instanceElement.Header.ID // ID from <Comp id="X">
			newEl.Header.PosX = instanceElement.Header.PosX
			newEl.Header.PosY = instanceElement.Header.PosY
			newEl.Header.Width = instanceElement.Header.Width   // from <Comp width=X>
			newEl.Header.Height = instanceElement.Header.Height // from <Comp height=Y>
			newEl.Header.Layout = instanceElement.Header.Layout // from <Comp layout=...> or its style

			if instanceElement.SourceElementName != "" { // Instance's name (often from its ID) takes precedence
				newEl.SourceElementName = instanceElement.SourceElementName
			}

			// StyleID for template root: Precedence is instance's style, then template's default.
			resolvedStyleIDForTplRoot := instanceElement.Header.StyleID // Style from <Comp style="X">
			if resolvedStyleIDForTplRoot == 0 && templateKrbHeader.StyleID != 0 {
				resolvedStyleIDForTplRoot = templateKrbHeader.StyleID // Style from Define Comp { RootElement { style: "Y" } }
			}
			newEl.Header.StyleID = resolvedStyleIDForTplRoot

			// Apply the resolved style to newEl (the template root)
			if style, found := findStyle(doc, newEl.Header.StyleID); found {
				r.applyStylePropertiesToElement(style.Properties, doc, newEl)
			}
			// Apply direct KRB properties from instanceElement's original KRB entry
			// These are standard KRB properties, not "Custom Properties" for component logic.
			if instanceElement.OriginalIndex >= 0 && instanceElement.OriginalIndex < len(doc.Properties) && len(doc.Properties[instanceElement.OriginalIndex]) > 0 {
				r.applyDirectPropertiesToElement(doc.Properties[instanceElement.OriginalIndex], doc, newEl)
			}
		} else { // This `newEl` is a non-root element *within* the template structure
			// Apply style from template's own definition (templateKrbHeader.StyleID)
			if templateChildStyle, templateChildStyleFound := findStyle(doc, templateKrbHeader.StyleID); templateChildStyleFound {
				r.applyStylePropertiesToElement(templateChildStyle.Properties, doc, newEl)
			}
			// Apply direct properties defined on this element *within the template itself*
			r.applyDirectPropertiesToElement(templateDirectProps, doc, newEl)
		}

		// Common post-style/direct-prop steps
		r.applyContextualDefaults(newEl)
		styleForNewEl, styleFoundForNewEl := findStyle(doc, newEl.Header.StyleID)
		r.resolveElementTextAndImage(doc, newEl, styleForNewEl, styleFoundForNewEl)

		// Event Handlers defined IN THE TEMPLATE (e.g. a static button inside a component)
		if templateKrbHeader.EventCount > 0 {
			eventDataSize := int(templateKrbHeader.EventCount) * krb.EventFileEntrySize
			eventBuf := make([]byte, eventDataSize)
			nEvent, errEvent := templateReader.Read(eventBuf)
			if errEvent != nil || nEvent < eventDataSize {
				return fmt.Errorf("expandComp '%s': read template event block for '%s': %w", compDefNameStr, newEl.SourceElementName, errEvent)
			}
			templateDataStreamOffset += uint32(nEvent)
			newEl.EventHandlers = make([]render.EventCallbackInfo, templateKrbHeader.EventCount)
			for k := uint8(0); k < templateKrbHeader.EventCount; k++ {
				offset := int(k) * krb.EventFileEntrySize
				eventType := krb.EventType(eventBuf[offset])
				callbackID := eventBuf[offset+1]
				if handlerName, ok := getStringValueByIdx(doc, callbackID); ok {
					newEl.EventHandlers[k] = render.EventCallbackInfo{EventType: eventType, HandlerName: handlerName}
				} else {
					log.Printf("Warn expandComponent: Template element '%s' (GlobalIdx %d) has invalid event callback string index %d.", newEl.SourceElementName, newEl.OriginalIndex, callbackID)
				}
			}
		}

		// Skip Animation Refs from template data stream
		if templateKrbHeader.AnimationCount > 0 {
			animRefDataSize := int(templateKrbHeader.AnimationCount) * krb.AnimationRefSize
			bytesSkipped, errAnim := templateReader.Seek(int64(animRefDataSize), io.SeekCurrent)
			if errAnim != nil || bytesSkipped < int64(animRefDataSize) {
				return fmt.Errorf("expandComp '%s': seek past template anim refs for '%s': %w", compDefNameStr, newEl.SourceElementName, errAnim)
			}
			templateDataStreamOffset += uint32(animRefDataSize)
		}

		// Read and store Child Ref info for Pass 2 (linking children within this template)
		if templateKrbHeader.ChildCount > 0 {
			tplChildRefs := make([]krb.ChildRef, templateKrbHeader.ChildCount)
			childRefDataSize := int(templateKrbHeader.ChildCount) * krb.ChildRefSize
			childRefBuf := make([]byte, childRefDataSize)
			nChildRef, errChildRef := templateReader.Read(childRefBuf)
			if errChildRef != nil || nChildRef < childRefDataSize {
				return fmt.Errorf("expandComp '%s': read template child refs for '%s': %w", compDefNameStr, newEl.SourceElementName, errChildRef)
			}
			templateDataStreamOffset += uint32(nChildRef)
			for k := uint8(0); k < templateKrbHeader.ChildCount; k++ {
				offset := int(k) * krb.ChildRefSize
				tplChildRefs[k] = krb.ChildRef{ChildOffset: krb.ReadU16LE(childRefBuf[offset : offset+krb.ChildRefSize])}
			}
			localTemplateChildInfos = append(localTemplateChildInfos, struct {
				parentGlobalIndex            int
				childRefs                    []krb.ChildRef
				parentHeaderOffsetInTemplate uint32
			}{
				parentGlobalIndex:            newElGlobalIndex,
				childRefs:                    tplChildRefs,
				parentHeaderOffsetInTemplate: currentElementHeaderOffsetInTemplate,
			})
		}

		// Recursive expansion for nested components defined within this template
		if nestedComponentNameForThisNewEl != "" {
			nestedCompDef := r.findComponentDefinition(nestedComponentNameForThisNewEl)
			if nestedCompDef != nil {
				log.Printf("Debug expandComponent: Recursively expanding NESTED component '%s' (placeholder is '%s', GlobalIdx %d) within outer component '%s'",
					nestedComponentNameForThisNewEl, newEl.SourceElementName, newEl.OriginalIndex, compDefNameStr)

				newEl.IsExpandedAsNestedComponent = true
				// `newEl` is the placeholder *within the current component's template* for the nested component.
				// KRY-usage children for a component defined *inside another component's template* are typically nil,
				// unless the KRY `Define` syntax allows passing children to such nested template components (advanced feature).
				err_nested := r.expandComponent(newEl, nestedCompDef, allElements, nextMasterIndex, nil /* No KRY-usage children for this nested instance */)
				if err_nested != nil {
					return fmt.Errorf("expandComponent '%s': failed during nested expansion of '%s' (for '%s'): %w", compDefNameStr, nestedComponentNameForThisNewEl, newEl.SourceElementName, err_nested)
				}
			} else {
				log.Printf("Warn expandComponent: Nested CompDef '%s' not found (for placeholder '%s', GlobalIdx %d in '%s').",
					nestedComponentNameForThisNewEl, newEl.SourceElementName, newEl.OriginalIndex, compDefNameStr)
			}
		}
	} // End PASS 1 (creating RenderElements from this template's data stream)

	log.Printf("Debug expandComponent [%s for %s]: Pass 1 created %d elements from template. nextMasterIndex is now: %d",
		compDefNameStr, instanceElement.SourceElementName, elementsCreatedInThisExpansionPass, *nextMasterIndex)

	// --- PASS 2: Link children *within* the structure just created from THIS template. ---
	// This loop iterates over the `localTemplateChildInfos` which were collected for elements defined *in this current template*.
	for _, info := range localTemplateChildInfos {
		// `parentElFromThisTemplate` is an element that was created from `compDef.RootElementTemplateData` in Pass 1.
		parentElFromThisTemplate := &(*allElements)[info.parentGlobalIndex]

		// If `parentElFromThisTemplate` was itself a placeholder for a *nested component* (e.g., `HabitTabBar` used inside `HabitHeader`),
		// its `Children` array would have been populated by the recursive call to `expandComponent` for that nested component.
		// In that case, we should NOT overwrite its children here with `childRefs` from its usage site *within the outer template*.
		// The `childRefs` for `parentElFromThisTemplate` (if it were a simple container in the template) define ITS children,
		// not children to be passed INTO it if it's a component instance.
		if parentElFromThisTemplate.IsExpandedAsNestedComponent { // Need a flag like this
			log.Printf("Debug expandComponent [%s for %s]: Template element '%s' (GlobalIdx %d) was expanded as a nested component. Skipping its direct child ref linking from this pass, its children are from its own template.",
				compDefNameStr, instanceElement.SourceElementName, parentElFromThisTemplate.SourceElementName, parentElFromThisTemplate.OriginalIndex)
			continue
		}
		// If newEl.IsExpandedAsNestedComponent is not available, use a heuristic:
		// if parentElFromThisTemplate.Children is already non-nil and non-empty,
		// and those children correctly point back to it, assume it was handled by a nested expansion.
		if parentElFromThisTemplate.Children != nil && len(parentElFromThisTemplate.Children) > 0 {
			isLikelyNestedExpansion := true
			for _, child := range parentElFromThisTemplate.Children {
				if child.Parent != parentElFromThisTemplate {
					isLikelyNestedExpansion = false
					break
				}
			}
			if isLikelyNestedExpansion {
				log.Printf("Debug expandComponent [%s for %s]: Template element '%s' (GlobalIdx %d) children already set (heuristic: likely nested expansion). Skipping its direct child ref linking from this pass.",
					compDefNameStr, instanceElement.SourceElementName, parentElFromThisTemplate.SourceElementName, parentElFromThisTemplate.OriginalIndex)
				continue
			}
		}

		// Initialize/clear children for `parentElFromThisTemplate` before linking its template-defined children.
		parentElFromThisTemplate.Children = make([]*render.RenderElement, 0, len(info.childRefs))

		for _, childRef := range info.childRefs {
			// `childRef.ChildOffset` is relative to `parentHeaderOffsetInTemplate` within the template data stream.
			childAbsoluteOffsetInTemplateStream := info.parentHeaderOffsetInTemplate + uint32(childRef.ChildOffset)
			childGlobalIndex, found := localTemplateOffsetToGlobalIndex[childAbsoluteOffsetInTemplateStream]
			if !found {
				log.Printf("Error expandComponent '%s': Child for template element '%s' (GlobalIdx %d) at template offset %d (abs %d) not found in local map.",
					compDefNameStr, parentElFromThisTemplate.SourceElementName, parentElFromThisTemplate.OriginalIndex, childRef.ChildOffset, childAbsoluteOffsetInTemplateStream)
				continue
			}
			childElFromThisTemplate := &(*allElements)[childGlobalIndex]

			// Link this child (from the template) to its parent (also from the template)
			childElFromThisTemplate.Parent = parentElFromThisTemplate
			parentElFromThisTemplate.Children = append(parentElFromThisTemplate.Children, childElFromThisTemplate)
		}
		log.Printf("Debug expandComponent [%s for %s]: Pass 2 linked %d children to template element '%s' (GlobalIdx %d).",
			compDefNameStr, instanceElement.SourceElementName, len(parentElFromThisTemplate.Children), parentElFromThisTemplate.SourceElementName, parentElFromThisTemplate.OriginalIndex)
	}

	// --- Finalize `instanceElement` (the original placeholder) ---
	// The `instanceElement` is "replaced" by the root of the expanded template.
	// Its `Children` should now point to the root element(s) created from this template expansion.
	if instanceElement != nil {
		if currentTemplateRootGlobalIndex != -1 { // Check if a template root was actually identified/created
			rootOfExpandedTemplate := &(*allElements)[currentTemplateRootGlobalIndex]

			// In Pass 1, rootOfExpandedTemplate.Parent was set to instanceElement.
			// Now, set instanceElement.Children to be this single root.
			instanceElement.Children = []*render.RenderElement{rootOfExpandedTemplate}
			log.Printf("Debug expandComponent [%s for %s]: Instance '%s' (GlobalIdx %d) children array now points to template root '%s' (GlobalIdx %d).",
				compDefNameStr, instanceElement.SourceElementName, instanceElement.SourceElementName, instanceElement.OriginalIndex, rootOfExpandedTemplate.SourceElementName, rootOfExpandedTemplate.OriginalIndex)
		} else {
			// This case (no template root from a non-empty template data) should be rare if parsing is correct.
			// If KRY-usage children were directly added earlier due to empty template, instanceElement.Children might already be set.
			log.Printf("Debug expandComponent [%s for %s]: No template root was identified for instance '%s'. Its children might be from KRY-usage or nil.",
				compDefNameStr, instanceElement.SourceElementName, instanceElement.SourceElementName)
			if instanceElement.Children == nil { // Ensure it's at least an empty slice
				instanceElement.Children = make([]*render.RenderElement, 0)
			}
		}
	}

	// --- Slot KRY-usage children into the expanded template structure ---
	if len(kryUsageChildren) > 0 {
		slotFound := false
		var slotElement *render.RenderElement

		// Search for the slot within the structure rooted at `instanceElement.Children[0]` (which is the template's root)
		if instanceElement != nil && len(instanceElement.Children) > 0 {
			searchStartNode := instanceElement.Children[0] // This is the root of the expanded template
			queue := []*render.RenderElement{searchStartNode}
			visitedInSearch := make(map[*render.RenderElement]bool)

			for len(queue) > 0 {
				currentNodeToSearch := queue[0]
				queue = queue[1:]
				if visitedInSearch[currentNodeToSearch] {
					continue
				}
				visitedInSearch[currentNodeToSearch] = true

				idNameFromTemplate, _ := getStringValueByIdx(doc, currentNodeToSearch.Header.ID) // ID is from template element
				if idNameFromTemplate == childrenSlotIDName {                                    // childrenSlotIDName is "children_host"
					slotElement = currentNodeToSearch
					slotFound = true
					break
				}
				if currentNodeToSearch.Children != nil {
					for _, childOfSearchNode := range currentNodeToSearch.Children {
						if !visitedInSearch[childOfSearchNode] {
							queue = append(queue, childOfSearchNode)
						}
					}
				}
			}
		}

		if slotFound && slotElement != nil {
			log.Printf("Debug expandComponent [%s for %s]: Found slot '%s' (GlobalIdx %d) in expanded template. Attaching %d KRY-usage children.",
				compDefNameStr, instanceElement.SourceElementName, childrenSlotIDName, slotElement.OriginalIndex, len(kryUsageChildren))
			if slotElement.Children == nil {
				slotElement.Children = make([]*render.RenderElement, 0, len(kryUsageChildren))
			}
			slotElement.Children = append(slotElement.Children, kryUsageChildren...)
			for _, kryChild := range kryUsageChildren {
				kryChild.Parent = slotElement // Re-parent KRY children to the slot
			}
		} else {
			log.Printf("Warn expandComponent [%s for %s]: No slot '%s' found in expanded template. Attempting to append %d KRY-usage children to first template root (if any and is container).",
				compDefNameStr, instanceElement.SourceElementName, childrenSlotIDName, len(kryUsageChildren))
			if instanceElement != nil && len(instanceElement.Children) > 0 {
				firstRootInTemplate := instanceElement.Children[0]
				// Only append if the template root is a type that can host children (e.g., Container)
				if firstRootInTemplate.Header.Type == krb.ElemTypeContainer { // Or other valid container types
					if firstRootInTemplate.Children == nil {
						firstRootInTemplate.Children = make([]*render.RenderElement, 0, len(kryUsageChildren))
					}
					firstRootInTemplate.Children = append(firstRootInTemplate.Children, kryUsageChildren...)
					for _, kryChild := range kryUsageChildren {
						kryChild.Parent = firstRootInTemplate
					}
				} else {
					log.Printf("Error expandComponent [%s for %s]: First template root '%s' (Type %X) is not a Container. Cannot append KRY-usage children when slot is missing.",
						compDefNameStr, instanceElement.SourceElementName, firstRootInTemplate.SourceElementName, firstRootInTemplate.Header.Type)
				}
			} else {
				log.Printf("Error expandComponent [%s for %s]: No template root to append KRY-usage children to (and no slot '%s' found). KRY children remain unparented from this instance.",
					instanceElement.SourceElementName, compDefNameStr, childrenSlotIDName)
			}
		}
	}

	log.Printf("Debug expandComponent: Finished expanding '%s' for instance '%s' (GlobalIdx %d). Final nextMasterIndex: %d",
		compDefNameStr, instanceElement.SourceElementName, instanceElement.OriginalIndex, *nextMasterIndex)
	return nil
}

func (r *RaylibRenderer) PerformLayout(
	el *render.RenderElement,
	parentContentX, parentContentY, parentContentW, parentContentH float32,
) {
	if el == nil {
		return
	}
	doc := r.docRef
	scale := r.scaleFactor

	elementIdentifier := el.SourceElementName
	if elementIdentifier == "" && el.Header.ID != 0 && doc != nil {
		idStr, _ := getStringValueByIdx(doc, el.Header.ID)
		if idStr != "" {
			elementIdentifier = idStr
		}
	}
	if elementIdentifier == "" {
		elementIdentifier = fmt.Sprintf("Type0x%X_Idx%d_NoName", el.Header.Type, el.OriginalIndex)
	}

	// Example: Enable detailed logging for specific elements if needed for debugging
	// isSpecificElementToLog := strings.Contains(elementIdentifier, "TabBar") || strings.Contains(elementIdentifier, "main_content_area")
	isSpecificElementToLog := false // Disable verbose logging by default

	if isSpecificElementToLog {
		log.Printf(
			">>>>> PerformLayout for: %s (Type:0x%X, OrigIdx:%d) ParentCTX:%.0f,%.0f,%.0f,%.0f",
			elementIdentifier, el.Header.Type, el.OriginalIndex, parentContentX, parentContentY, parentContentW, parentContentH,
		)
		log.Printf(
			"      Hdr: W:%d,H:%d,PosX:%d,PosY:%d,Layout:0x%02X(Abs:%t,Grow:%t)",
			el.Header.Width, el.Header.Height, el.Header.PosX, el.Header.PosY,
			el.Header.Layout, el.Header.LayoutAbsolute(), el.Header.LayoutGrow(),
		)
	}

	isRootElement := (el.Parent == nil)
	scaledUint16Local := func(v uint16) float32 { return float32(v) * scale }

	// --- Step 1: Determine EXPLICIT Size ---
	// Priority:
	// 1. Direct KRB Header Width/Height (from KRY <Element width=X height=Y>)
	// 2. Direct KRB Property (from KRY width: Z or width: "Z%")
	// 3. Style Property (from KRY style "s" { width: A or width: "A%" })

	hasExplicitWidth := false
	desiredWidth := float32(0.0)
	if el.Header.Width > 0 { // From KRY <Element width=X> (direct KRB header)
		desiredWidth = scaledUint16Local(el.Header.Width) // KRB Header W/H are direct pixel values
		hasExplicitWidth = true
	}

	hasExplicitHeight := false
	desiredHeight := float32(0.0)
	if el.Header.Height > 0 { // From KRY <Element height=X> (direct KRB header)
		desiredHeight = scaledUint16Local(el.Header.Height) // KRB Header W/H are direct pixel values
		hasExplicitHeight = true
	}

	// Check direct KRB properties (e.g., from KRY width: "50%" or width: 100)
	// These override KRB Header Width/Height if both are present (though KRB spec implies header W/H might be max values).
	// For now, assume direct KRB property takes precedence if it exists and is valid.
	if doc != nil && el.OriginalIndex >= 0 && el.OriginalIndex < len(doc.Properties) && doc.Properties[el.OriginalIndex] != nil {
		elementDirectProps := doc.Properties[el.OriginalIndex]
		// Width from direct KRB property
		propWVal, propWType, _, propWErr := getNumericValueForSizeProp(elementDirectProps, krb.PropIDMaxWidth, doc)
		if propWErr == nil {
			explicitPropWidth := MuxFloat32(propWType == krb.ValTypePercentage, (propWVal/256.0)*parentContentW, propWVal*scale)
			if explicitPropWidth > 0 { // A valid direct prop width was found
				desiredWidth = explicitPropWidth
				hasExplicitWidth = true
			}
		}
		// Height from direct KRB property
		propHVal, propHType, _, propHErr := getNumericValueForSizeProp(elementDirectProps, krb.PropIDMaxHeight, doc)
		if propHErr == nil {
			explicitPropHeight := MuxFloat32(propHType == krb.ValTypePercentage, (propHVal/256.0)*parentContentH, propHVal*scale)
			if explicitPropHeight > 0 { // A valid direct prop height was found
				desiredHeight = explicitPropHeight
				hasExplicitHeight = true
			}
		}
	}

	// Check element's resolved style for size properties IF NOT ALREADY EXPLICITLY SET by header or direct KRB prop.
	if !hasExplicitWidth {
		style, styleFound := findStyle(doc, el.Header.StyleID)
		if styleFound {
			prop, propFound := getStylePropertyValue(style, krb.PropIDMaxWidth) // KRY 'width' property in style maps to MaxWidth
			if propFound {
				val, valType, _, err := getNumericValueFromKrbProp(prop, doc)
				if err == nil {
					styledWidth := MuxFloat32(valType == krb.ValTypePercentage, (val/256.0)*parentContentW, val*scale)
					if styledWidth > 0 {
						desiredWidth = styledWidth
						hasExplicitWidth = true
						if isSpecificElementToLog {
							log.Printf("      S1 - Styled Width for %s: %.1f (from prop value %.1f, type %d, StyleID %d)", elementIdentifier, desiredWidth, val, valType, el.Header.StyleID)
						}
					}
				}
			}
		}
	}

	if !hasExplicitHeight {
		style, styleFound := findStyle(doc, el.Header.StyleID)
		if styleFound {
			prop, propFound := getStylePropertyValue(style, krb.PropIDMaxHeight) // KRY 'height' property in style maps to MaxHeight
			if propFound {
				val, valType, _, err := getNumericValueFromKrbProp(prop, doc)
				if err == nil {
					styledHeight := MuxFloat32(valType == krb.ValTypePercentage, (val/256.0)*parentContentH, val*scale)
					if styledHeight > 0 {
						desiredHeight = styledHeight
						hasExplicitHeight = true
						if isSpecificElementToLog {
							log.Printf("      S1 - Styled Height for %s: %.1f (from prop value %.1f, type %d, StyleID %d)", elementIdentifier, desiredHeight, val, valType, el.Header.StyleID)
						}
					}
				}
			}
		}
	}

	if isSpecificElementToLog {
		log.Printf("      S1 - After All Explicit Size Checks for %s: W:%.1f(exp:%t), H:%.1f(exp:%t)", elementIdentifier, desiredWidth, hasExplicitWidth, desiredHeight, hasExplicitHeight)
	}

	// --- Step 2: Apply INTRINSIC and DEFAULT SIZING (if not explicitly sized) ---
	hPadding := ScaledF32(el.Padding[1], scale) + ScaledF32(el.Padding[3], scale)
	vPadding := ScaledF32(el.Padding[0], scale) + ScaledF32(el.Padding[2], scale)
	hBorder := ScaledF32(el.BorderWidths[1], scale) + ScaledF32(el.BorderWidths[3], scale) // Sum of left and right border
	vBorder := ScaledF32(el.BorderWidths[0], scale) + ScaledF32(el.BorderWidths[2], scale) // Sum of top and bottom border

	isGrow := el.Header.LayoutGrow()
	isAbsolute := el.Header.LayoutAbsolute()

	if (el.Header.Type == krb.ElemTypeText || el.Header.Type == krb.ElemTypeButton) && el.Text != "" {
		// Determine font size (TODO: this needs to come from resolved font size, not just base)
		// For now, using baseFontSize for simplicity in this context
		// In a full system, el.ResolvedFontSize would be set by style/direct/inheritance pass
		finalFontSizePixels := MaxF(1.0, baseFontSize*scale) // Example

		if !hasExplicitWidth {
			textWidthMeasuredInPixels := float32(rl.MeasureText(el.Text, int32(finalFontSizePixels)))
			// Intrinsic width includes text + horizontal padding + horizontal border
			desiredWidth = textWidthMeasuredInPixels + hPadding + hBorder
			if isSpecificElementToLog {
				log.Printf("      S2a - Intrinsic W (Text) for %s: %.1f (text:%.1f, hPad:%.1f, hBorder:%.1f)", elementIdentifier, desiredWidth, textWidthMeasuredInPixels, hPadding, hBorder)
			}
		}
		if !hasExplicitHeight {
			textHeightMeasuredInPixels := finalFontSizePixels
			// Intrinsic height includes text + vertical padding + vertical border
			desiredHeight = textHeightMeasuredInPixels + vPadding + vBorder
			if isSpecificElementToLog {
				log.Printf("      S2a - Intrinsic H (Text) for %s: %.1f (text:%.1f, vPad:%.1f, vBorder:%.1f)", elementIdentifier, desiredHeight, textHeightMeasuredInPixels, vPadding, vBorder)
			}
		}
	} else if el.Header.Type == krb.ElemTypeImage && el.ResourceIndex != render.InvalidResourceIndex {
		texWidthPx := float32(0)
		texHeightPx := float32(0)
		if el.TextureLoaded && el.Texture.ID > 0 {
			texWidthPx = float32(el.Texture.Width)
			texHeightPx = float32(el.Texture.Height)
		}
		if !hasExplicitWidth {
			desiredWidth = texWidthPx*scale + hPadding + hBorder
			if isSpecificElementToLog {
				log.Printf("      S2b - Intrinsic W (Image) for %s: %.1f (texW_native:%.1f, scale:%.1f, hPad:%.1f, hBorder:%.1f)", elementIdentifier, desiredWidth, texWidthPx, scale, hPadding, hBorder)
			}
		}
		if !hasExplicitHeight {
			desiredHeight = texHeightPx*scale + vPadding + vBorder
			if isSpecificElementToLog {
				log.Printf("      S2b - Intrinsic H (Image) for %s: %.1f (texH_native:%.1f, scale:%.1f, vPad:%.1f, vBorder:%.1f)", elementIdentifier, desiredHeight, texHeightPx, scale, vPadding, vBorder)
			}
		}
	}

	// Default sizing for containers/app if no explicit/intrinsic size and not growing/absolute
	if !hasExplicitWidth && !isGrow && !isAbsolute {
		if desiredWidth == 0 && (el.Header.Type == krb.ElemTypeContainer || el.Header.Type == krb.ElemTypeApp) {
			desiredWidth = parentContentW // Default to fill parent's content width
			if isSpecificElementToLog {
				log.Printf("      S2c - Default W (Container/App) for %s: %.1f from parent content area", elementIdentifier, desiredWidth)
			}
		}
	}
	if !hasExplicitHeight && !isGrow && !isAbsolute {
		if desiredHeight == 0 && (el.Header.Type == krb.ElemTypeContainer || el.Header.Type == krb.ElemTypeApp) {
			desiredHeight = parentContentH // Default to fill parent's content height
			if isSpecificElementToLog {
				log.Printf("      S2c - Default H (Container/App) for %s: %.1f from parent content area", elementIdentifier, desiredHeight)
			}
		}
	}

	// Assign RenderW/H based on findings
	if isRootElement {
		el.RenderW = MuxFloat32(hasExplicitWidth, desiredWidth, parentContentW)
		el.RenderH = MuxFloat32(hasExplicitHeight, desiredHeight, parentContentH)
	} else {
		el.RenderW = MaxF(0, desiredWidth)  // Cannot be negative
		el.RenderH = MaxF(0, desiredHeight) // Cannot be negative
	}

	if isSpecificElementToLog {
		log.Printf("      S2 - Assigned RenderW/H for %s: W:%.1f, H:%.1f", elementIdentifier, el.RenderW, el.RenderH)
	}

	// --- Step 3: Determine Base Render Position ---
	if el.Header.LayoutAbsolute() {
		offsetX := scaledUint16Local(el.Header.PosX)
		offsetY := scaledUint16Local(el.Header.PosY)
		if el.Parent != nil {
			el.RenderX = el.Parent.RenderX + offsetX // Relative to parent's origin
			el.RenderY = el.Parent.RenderY + offsetY
		} else { // Should not happen for absolute if not root, but as fallback
			el.RenderX = parentContentX + offsetX // Relative to parent's content area origin
			el.RenderY = parentContentY + offsetY
		}
	} else { // Flow layout
		el.RenderX = parentContentX // Initial position before flow adjustments by PerformLayoutChildren
		el.RenderY = parentContentY
	}

	if isSpecificElementToLog {
		log.Printf("      S3 - Initial Position for %s: X:%.1f, Y:%.1f (Abs:%t)", elementIdentifier, el.RenderX, el.RenderY, el.Header.LayoutAbsolute())
	}

	// --- Step 4: Calculate Content Area for Children ---
	// This uses the *current* el.RenderW/H which might be adjusted by PerformLayoutChildren if content hugging occurs.
	// For now, calculate based on current el.RenderW/H.
	childPaddingTop := ScaledF32(el.Padding[0], scale)
	childPaddingRight := ScaledF32(el.Padding[1], scale)
	childPaddingBottom := ScaledF32(el.Padding[2], scale)
	childPaddingLeft := ScaledF32(el.Padding[3], scale)
	childBorderTop := ScaledF32(el.BorderWidths[0], scale)
	childBorderRight := ScaledF32(el.BorderWidths[1], scale)
	childBorderBottom := ScaledF32(el.BorderWidths[2], scale)
	childBorderLeft := ScaledF32(el.BorderWidths[3], scale)

	// childContentAreaX/Y are absolute screen coordinates for where children's layout context begins
	childContentAreaX := el.RenderX + childBorderLeft + childPaddingLeft
	childContentAreaY := el.RenderY + childBorderTop + childPaddingTop
	// childAvailableWidth/Height is the space *within* this element for its children to flow
	childAvailableWidth := el.RenderW - (childBorderLeft + childBorderRight + childPaddingLeft + childPaddingRight)
	childAvailableHeight := el.RenderH - (childBorderTop + childBorderBottom + childPaddingTop + childPaddingBottom)
	childAvailableWidth = MaxF(0, childAvailableWidth)   // Ensure non-negative
	childAvailableHeight = MaxF(0, childAvailableHeight) // Ensure non-negative

	if isSpecificElementToLog {
		log.Printf("      S4 - Child Content Area for %s (abs origin: X:%.1f, Y:%.1f. available W:%.1f, H:%.1f)",
			elementIdentifier, childContentAreaX, childContentAreaY, childAvailableWidth, childAvailableHeight)
	}

	// --- Step 5 & 6: Layout Children & Content Hugging ---
	if len(el.Children) > 0 && !el.Header.LayoutAbsolute() { // Absolute positioned elements don't manage flow of their children in this model
		if isSpecificElementToLog {
			log.Printf("      S5 - Calling PerformLayoutChildren for %s...", elementIdentifier)
		}
		// This call will position children within childContentAreaX/Y using childAvailableWidth/Height
		r.PerformLayoutChildren(el, childContentAreaX, childContentAreaY, childAvailableWidth, childAvailableHeight)

		// Content Hugging: If element has no explicit height and is not set to grow, adjust its height to fit children.
		// This is a simplified version. A full implementation would need to consider layout direction more deeply.
		if !isRootElement && !hasExplicitHeight && !isGrow {
			actualChildrenMaxY := float32(0)
			if el.Header.LayoutDirection() == krb.LayoutDirColumn || el.Header.LayoutDirection() == krb.LayoutDirColumnReverse {
				// For column layout, sum heights of flow children + gaps
				currentYPos := float32(0)
				numFlowChildren := 0
				gapVal := float32(0) // Simplified: get actual gap
				for _, child := range el.Children {
					if child != nil && !child.Header.LayoutAbsolute() {
						if numFlowChildren > 0 {
							currentYPos += gapVal
						}
						currentYPos += child.RenderH
						numFlowChildren++
					}
				}
				actualChildrenMaxY = currentYPos
			} else { // For row layout (or other), find max Y extent of children relative to childContentAreaY
				for _, child := range el.Children {
					if child != nil && !child.Header.LayoutAbsolute() {
						childBottomYRelativeToContentArea := (child.RenderY - childContentAreaY) + child.RenderH
						if childBottomYRelativeToContentArea > actualChildrenMaxY {
							actualChildrenMaxY = childBottomYRelativeToContentArea
						}
					}
				}
			}

			// If children dictate a height, and it's different from current desiredHeight (which might be 0 or from intrinsic text/image)
			if actualChildrenMaxY > 0 {
				newHeightFromChildren := actualChildrenMaxY + vPadding + vBorder // Add back own padding and border
				// Only hug if it makes sense (e.g. if children define a larger space than intrinsic, or if intrinsic was 0)
				// Or if current RenderH is larger than needed (e.g. a container was given parent height but children are smaller)
				if el.RenderH == 0 || newHeightFromChildren > el.RenderH || (el.RenderH > newHeightFromChildren && (el.Header.Type == krb.ElemTypeContainer || el.Header.Type == krb.ElemTypeApp)) {
					el.RenderH = newHeightFromChildren
					if isSpecificElementToLog {
						log.Printf("      S6 - Content Hug/Shrink H for %s: %.1f", elementIdentifier, el.RenderH)
					}
					// Recalculate childAvailableHeight if parent height changed due to hugging
					childAvailableHeight = el.RenderH - (vBorder + vPadding)
					childAvailableHeight = MaxF(0, childAvailableHeight)
					// OPTIONAL: Re-run PerformLayoutChildren if parent height changed and children might need to re-flow/re-align in new space
					// For simplicity, not doing a full re-layout pass here, but a robust engine might.
				}
			}
		}
	} else if len(el.Children) > 0 && el.Header.LayoutAbsolute() {
		// For absolute positioned parents, their children are also laid out relative to parent's origin,
		// but within the parent's bounds (passed as parentContentX/Y/W/H to PerformLayout).
		for _, child := range el.Children {
			// Each child (absolute or flow) of an absolute parent is laid out starting from the parent's (X,Y)
			// using parent's (W,H) as the available space.
			r.PerformLayout(child, el.RenderX, el.RenderY, el.RenderW, el.RenderH)
		}
	}

	if isSpecificElementToLog {
		log.Printf("      S5/6 - After Children/Hugging for %s: W:%.1f, H:%.1f, X:%.1f, Y:%.1f",
			elementIdentifier, el.RenderW, el.RenderH, el.RenderX, el.RenderY)
	}

	// --- Step 7: Apply Min/Max-Width/Height Constraints (from direct KRB properties) ---
	// MaxWidth/MaxHeight were already considered in Step 1 from direct KRB props.
	// Here, we apply MinWidth/MinHeight.
	if doc != nil && el.OriginalIndex >= 0 && el.OriginalIndex < len(doc.Properties) && doc.Properties[el.OriginalIndex] != nil {
		elementDirectProps := doc.Properties[el.OriginalIndex]
		minWVal, minWType, _, minWErr := getNumericValueForSizeProp(elementDirectProps, krb.PropIDMinWidth, doc)
		if minWErr == nil {
			minWidthConstraint := MuxFloat32(minWType == krb.ValTypePercentage, (minWVal/256.0)*parentContentW, minWVal*scale)
			if minWidthConstraint > 0 && el.RenderW < minWidthConstraint {
				el.RenderW = minWidthConstraint
			}
		}
		minHVal, minHType, _, minHErr := getNumericValueForSizeProp(elementDirectProps, krb.PropIDMinHeight, doc)
		if minHErr == nil {
			minHeightConstraint := MuxFloat32(minHType == krb.ValTypePercentage, (minHVal/256.0)*parentContentH, minHVal*scale)
			if minHeightConstraint > 0 && el.RenderH < minHeightConstraint {
				el.RenderH = minHeightConstraint
			}
		}
	}

	if isSpecificElementToLog {
		log.Printf("      S7 - Min Constraints Applied for %s: W:%.1f, H:%.1f", elementIdentifier, el.RenderW, el.RenderH)
	}

	// --- Step 8: Final Fallback for Zero Size (as per spec 3.1) ---
	el.RenderW = MaxF(0, el.RenderW) // Ensure non-negative
	el.RenderH = MaxF(0, el.RenderH) // Ensure non-negative

	// If an element is intended to be visible but ended up with zero height (and has width)
	if el.RenderW > 0 && el.RenderH == 0 {
		isConsideredVisibleDueToContentOrStyle := el.Header.Type == krb.ElemTypeContainer ||
			el.Header.Type == krb.ElemTypeApp ||
			el.BgColor.A > 0 ||
			(el.BorderWidths[0]+el.BorderWidths[1]+el.BorderWidths[2]+el.BorderWidths[3] > 0) ||
			((el.Header.Type == krb.ElemTypeText || el.Header.Type == krb.ElemTypeButton) && el.Text != "") // Text element with text

		if isConsideredVisibleDueToContentOrStyle {
			// Default to a scaled base font size or 1.0 * scaleFactor if font size is also zero
			minVisibleDim := MaxF(baseFontSize*scale, 1.0*scale)
			el.RenderH = minVisibleDim
			if isSpecificElementToLog {
				log.Printf("      S8 - Fallback Zero H for %s: %.1f applied (min visible dimension)", elementIdentifier, el.RenderH)
			}
		}
	}
	// Symmetrically for width
	if el.RenderH > 0 && el.RenderW == 0 {
		isConsideredVisibleDueToContentOrStyle := el.Header.Type == krb.ElemTypeContainer ||
			el.Header.Type == krb.ElemTypeApp ||
			el.BgColor.A > 0 ||
			(el.BorderWidths[0]+el.BorderWidths[1]+el.BorderWidths[2]+el.BorderWidths[3] > 0) ||
			((el.Header.Type == krb.ElemTypeText || el.Header.Type == krb.ElemTypeButton) && el.Text != "")

		if isConsideredVisibleDueToContentOrStyle {
			minVisibleDim := MaxF(baseFontSize*scale, 1.0*scale)
			el.RenderW = minVisibleDim
			if isSpecificElementToLog {
				log.Printf("      S8 - Fallback Zero W for %s: %.1f applied (min visible dimension)", elementIdentifier, el.RenderW)
			}
		}
	}
	el.RenderW = MaxF(0, el.RenderW) // Final clamp after potential fallback
	el.RenderH = MaxF(0, el.RenderH) // Final clamp

	if isSpecificElementToLog {
		log.Printf(
			"<<<<< PerformLayout END for: %s -- Final Render: X:%.1f,Y:%.1f, W:%.1f,H:%.1f",
			elementIdentifier, el.RenderX, el.RenderY, el.RenderW, el.RenderH,
		)
	}
}

func (r *RaylibRenderer) PerformLayoutChildren(
	parent *render.RenderElement,
	parentClientOriginX, parentClientOriginY,
	availableClientWidth, availableClientHeight float32,
) {

	if parent == nil || len(parent.Children) == 0 {
		return
	}
	doc := r.docRef
	scale := r.scaleFactor

	parentIdentifier := parent.SourceElementName

	if parentIdentifier == "" {
		parentIdentifier = fmt.Sprintf("ParentType0x%X_Idx%d", parent.Header.Type, parent.OriginalIndex)
	}

	//isParentSpecificToLog := strings.Contains(parentIdentifier, "HelloWidget") || parentIdentifier == "Type0x0_Idx0"
	isParentSpecificToLog := false
	if isParentSpecificToLog {
		log.Printf(
			">>>>> PerformLayoutChildren for PARENT: %s (ContentOrigin: X:%.0f,Y:%.0f, AvailW:%.0f,AvailH:%.0f, LayoutByte:0x%02X)",
			parentIdentifier, parentClientOriginX, parentClientOriginY, availableClientWidth, availableClientHeight, parent.Header.Layout,
		)
	}

	flowChildren := make([]*render.RenderElement, 0, len(parent.Children))
	absoluteChildren := make([]*render.RenderElement, 0)

	for _, child := range parent.Children {

		if child != nil {

			if child.Header.LayoutAbsolute() {
				absoluteChildren = append(absoluteChildren, child)
			} else {
				flowChildren = append(flowChildren, child)
			}
		}
	}

	scaledUint16Local := func(v uint16) float32 { return float32(v) * scale }

	// --- Layout Flow Children ---
	if len(flowChildren) > 0 {
		layoutDirection := parent.Header.LayoutDirection()
		layoutAlignment := parent.Header.LayoutAlignment()
		crossAxisAlignment := parent.Header.LayoutCrossAlignment()
		isLayoutReversed := (layoutDirection == krb.LayoutDirRowReverse || layoutDirection == krb.LayoutDirColumnReverse)
		isMainAxisHorizontal := (layoutDirection == krb.LayoutDirRow || layoutDirection == krb.LayoutDirRowReverse)

		gapValue := float32(0)

		if parentStyle, styleFound := findStyle(doc, parent.Header.StyleID); styleFound {

			if gapProp, propFound := getStylePropertyValue(parentStyle, krb.PropIDGap); propFound {

				if gVal, valOk := getShortValue(gapProp); valOk {
					gapValue = float32(gVal) * scale
				}
			}
		}

		if doc != nil && parent.OriginalIndex < len(doc.Properties) && len(doc.Properties[parent.OriginalIndex]) > 0 {

			for _, prop := range doc.Properties[parent.OriginalIndex] {

				if prop.ID == krb.PropIDGap {

					if gVal, valOk := getShortValue(&prop); valOk {
						gapValue = float32(gVal) * scale
						break
					}
				}
			}
		}

		totalGapSpace := float32(0)

		if len(flowChildren) > 1 {
			totalGapSpace = gapValue * float32(len(flowChildren)-1)
		}

		mainAxisEffectiveSpaceForParentLayout := MuxFloat32(isMainAxisHorizontal, availableClientWidth, availableClientHeight)
		mainAxisEffectiveSpaceForElements := MaxF(0, mainAxisEffectiveSpaceForParentLayout-totalGapSpace)
		crossAxisEffectiveSizeForParentLayout := MuxFloat32(isMainAxisHorizontal, availableClientHeight, availableClientWidth)

		// Pass 1: Sizing
		for _, child := range flowChildren {

			if isParentSpecificToLog {
				log.Printf("      PLC Pass 1 (Sizing) - Calling PerformLayout for child: %s", child.SourceElementName)
			}
			r.PerformLayout(child, parentClientOriginX, parentClientOriginY, availableClientWidth, availableClientHeight)
		}

		// Pass 2: Calculate fixed size and grow children
		totalFixedSizeOnMainAxis := float32(0)
		numberOfGrowChildren := 0

		for _, child := range flowChildren {

			if child.Header.LayoutGrow() {
				numberOfGrowChildren++
			} else {
				totalFixedSizeOnMainAxis += MuxFloat32(isMainAxisHorizontal, child.RenderW, child.RenderH)
			}
		}
		totalFixedSizeOnMainAxis = MaxF(0, totalFixedSizeOnMainAxis)

		spaceAvailableForGrowingChildren := MaxF(0, mainAxisEffectiveSpaceForElements-totalFixedSizeOnMainAxis)
		sizePerGrowChild := float32(0)

		if numberOfGrowChildren > 0 && spaceAvailableForGrowingChildren > 0 {
			sizePerGrowChild = spaceAvailableForGrowingChildren / float32(numberOfGrowChildren)
		}

		// Pass 3: Apply grow and cross-axis stretch
		totalFinalElementSizeOnMainAxis := float32(0)

		for _, child := range flowChildren {

			if child.Header.LayoutGrow() && sizePerGrowChild > 0 {

				if isMainAxisHorizontal {
					child.RenderW = sizePerGrowChild
				} else {
					child.RenderH = sizePerGrowChild
				}

				if isParentSpecificToLog {
					log.Printf(
						"      PLC Pass 3 (Grow) - Child %s grew to main-axis size: %.1f",
						child.SourceElementName, MuxFloat32(isMainAxisHorizontal, child.RenderW, child.RenderH),
					)
				}
			}

			if crossAxisAlignment == krb.LayoutAlignStretch {

				if isMainAxisHorizontal {

					if child.Header.Height == 0 && child.RenderH < crossAxisEffectiveSizeForParentLayout {
						child.RenderH = crossAxisEffectiveSizeForParentLayout

						if isParentSpecificToLog {
							log.Printf("      PLC Pass 3 (Stretch) - Child %s stretched H to %.1f", child.SourceElementName, child.RenderH)
						}
					}
				} else {

					if child.Header.Width == 0 && child.RenderW < crossAxisEffectiveSizeForParentLayout {
						child.RenderW = crossAxisEffectiveSizeForParentLayout

						if isParentSpecificToLog {
							log.Printf("      PLC Pass 3 (Stretch) - Child %s stretched W to %.1f", child.SourceElementName, child.RenderW)
						}
					}
				}
			}
			child.RenderW = MaxF(0, child.RenderW)
			child.RenderH = MaxF(0, child.RenderH)
			totalFinalElementSizeOnMainAxis += MuxFloat32(isMainAxisHorizontal, child.RenderW, child.RenderH)
		}

		totalUsedSpaceWithGaps := totalFinalElementSizeOnMainAxis + totalGapSpace
		startOffsetOnMainAxis, effectiveSpacingBetweenItems := calculateAlignmentOffsetsF(
			layoutAlignment,
			mainAxisEffectiveSpaceForParentLayout,
			totalUsedSpaceWithGaps,
			len(flowChildren), isLayoutReversed, gapValue,
		)

		if isParentSpecificToLog {
			log.Printf("      PLC Details: mainEffSpaceForElems:%.0f, crossEffSizeForParent:%.0f", mainAxisEffectiveSpaceForElements, crossAxisEffectiveSizeForParentLayout)
			log.Printf("      PLC Details: totalFixed:%.0f, numGrow:%d, spaceForGrow:%.0f, sizePerGrow:%.0f", totalFixedSizeOnMainAxis, numberOfGrowChildren, spaceAvailableForGrowingChildren, sizePerGrowChild)
			log.Printf("      PLC Details: totalFinalMainAxis:%.0f, totalUsedWithGaps:%.0f", totalFinalElementSizeOnMainAxis, totalUsedSpaceWithGaps)
			log.Printf("      PLC Details: startOffMain:%.0f, effSpacing:%.0f", startOffsetOnMainAxis, effectiveSpacingBetweenItems)
		}

		// Pass 4: Position and recurse
		currentMainAxisPosition := startOffsetOnMainAxis
		childOrderIndices := make([]int, len(flowChildren))

		for i := range childOrderIndices {
			childOrderIndices[i] = i
		}

		if isLayoutReversed {
			ReverseSliceInt(childOrderIndices)
		}

		for i, orderedChildIndex := range childOrderIndices {
			child := flowChildren[orderedChildIndex]
			childMainAxisSizeValue := MuxFloat32(isMainAxisHorizontal, child.RenderW, child.RenderH)
			childCrossAxisSizeValue := MuxFloat32(isMainAxisHorizontal, child.RenderH, child.RenderW)
			crossAxisOffset := calculateCrossAxisOffsetF(crossAxisAlignment, crossAxisEffectiveSizeForParentLayout, childCrossAxisSizeValue)

			if isMainAxisHorizontal {
				child.RenderX = parentClientOriginX + currentMainAxisPosition
				child.RenderY = parentClientOriginY + crossAxisOffset
			} else {
				child.RenderX = parentClientOriginX + crossAxisOffset
				child.RenderY = parentClientOriginY + currentMainAxisPosition
			}

			if !child.Header.LayoutAbsolute() && (child.Header.PosX != 0 || child.Header.PosY != 0) {
				childOwnOffsetX := scaledUint16Local(child.Header.PosX)
				childOwnOffsetY := scaledUint16Local(child.Header.PosY)
				child.RenderX += childOwnOffsetX
				child.RenderY += childOwnOffsetY
				if isParentSpecificToLog || child.SourceElementName == "Type0x1_Idx1" {
					log.Printf("      PLC Pass 4 - Child %s applied its own PosX/Y offset: dX:%.1f, dY:%.1f. New pos: X:%.1f,Y:%.1f",
						child.SourceElementName, childOwnOffsetX, childOwnOffsetY, child.RenderX, child.RenderY)
				}
			}

			if isParentSpecificToLog {
				log.Printf(
					"      PLC Pass 4 - Positioned Child %s: Final X:%.0f,Y:%.0f (Child W:%.0f,H:%.0f)",
					child.SourceElementName, child.RenderX, child.RenderY, child.RenderW, child.RenderH,
				)
			}

			if len(child.Children) > 0 {
				childPaddingTop := ScaledF32(child.Padding[0], scale)
				childPaddingRight := ScaledF32(child.Padding[1], scale)
				childPaddingBottom := ScaledF32(child.Padding[2], scale)
				childPaddingLeft := ScaledF32(child.Padding[3], scale)
				childBorderTop := ScaledF32(child.BorderWidths[0], scale)
				childBorderRight := ScaledF32(child.BorderWidths[1], scale)
				childBorderBottom := ScaledF32(child.BorderWidths[2], scale)
				childBorderLeft := ScaledF32(child.BorderWidths[3], scale)

				grandChildContentAreaX := child.RenderX + childBorderLeft + childPaddingLeft
				grandChildContentAreaY := child.RenderY + childBorderTop + childPaddingTop
				grandChildAvailableWidth := child.RenderW - (childBorderLeft + childBorderRight + childPaddingLeft + childPaddingRight)
				grandChildAvailableHeight := child.RenderH - (childBorderTop + childBorderBottom + childPaddingTop + childPaddingBottom)
				grandChildAvailableWidth = MaxF(0, grandChildAvailableWidth)
				grandChildAvailableHeight = MaxF(0, grandChildAvailableHeight)

				r.PerformLayoutChildren(child, grandChildContentAreaX, grandChildContentAreaY, grandChildAvailableWidth, grandChildAvailableHeight)
			}

			currentMainAxisPosition += childMainAxisSizeValue

			if i < len(flowChildren)-1 {
				currentMainAxisPosition += effectiveSpacingBetweenItems
			}
		}
	}

	// --- Layout Absolute Children ---
	if len(absoluteChildren) > 0 {

		for _, child := range absoluteChildren {

			if isParentSpecificToLog {
				log.Printf(
					"      PLC - Calling PerformLayout for Absolute Child: %s (Parent Frame: X:%.0f,Y:%.0f W:%.0f,H:%.0f)",
					child.SourceElementName, parent.RenderX, parent.RenderY, parent.RenderW, parent.RenderH,
				)
			}
			r.PerformLayout(child, parent.RenderX, parent.RenderY, parent.RenderW, parent.RenderH)
		}
	}

	if isParentSpecificToLog {
		log.Printf("<<<<< PerformLayoutChildren END for PARENT: %s", parentIdentifier)
	}
}

func getStringValueByIdxFallback(doc *krb.Document, idx uint8, fallback string) string {
	s, ok := getStringValueByIdx(doc, idx)

	if !ok {
		return fallback
	}
	return s
}
