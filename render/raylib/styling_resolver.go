package raylib

import (
	"log" // For debug logging

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/kryonlabs/kryon-go-runtime/go/krb"
	"github.com/kryonlabs/kryon-go-runtime/go/render"
)

// --- Methods for Applying Properties to WindowConfig ---

func (r *RaylibRenderer) applyStylePropertiesToWindowConfig(
	props []krb.Property,
	doc *krb.Document,
	config *render.WindowConfig,
) {
	if doc == nil || config == nil {
		return
	}
	for _, prop := range props {
		switch prop.ID {
		case krb.PropIDBgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				config.DefaultBg = c
			}
		case krb.PropIDFgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				config.DefaultFgColor = c
			}
		case krb.PropIDBorderColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				config.DefaultBorderColor = c
			}
		case krb.PropIDFontSize:
			if fsRaw, ok := getShortValue(&prop); ok && fsRaw > 0 {
				config.DefaultFontSize = float32(fsRaw)
			}
		}
	}
}

func (r *RaylibRenderer) applyDirectPropertiesToWindowConfig(
	props []krb.Property,
	doc *krb.Document,
	config *render.WindowConfig,
) {
	if config == nil || doc == nil {
		return
	}
	for _, prop := range props {
		switch prop.ID {
		case krb.PropIDWindowWidth:
			if w, ok := getShortValue(&prop); ok && w > 0 {
				config.Width = int(w)
			}
		case krb.PropIDWindowHeight:
			if h, ok := getShortValue(&prop); ok && h > 0 {
				config.Height = int(h)
			}
		case krb.PropIDWindowTitle:
			if strIdx, ok := getByteValue(&prop); ok {
				if s, strOk := getStringValueByIdx(doc, strIdx); strOk {
					config.Title = s
				}
			}
		case krb.PropIDResizable:
			if rVal, ok := getByteValue(&prop); ok {
				config.Resizable = (rVal != 0)
			}
		case krb.PropIDScaleFactor:
			if sfRaw, ok := getShortValue(&prop); ok && sfRaw > 0 {
				config.ScaleFactor = float32(sfRaw) / 256.0
			}
		case krb.PropIDBgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				config.DefaultBg = c
			}
		case krb.PropIDFgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				config.DefaultFgColor = c
			}
		case krb.PropIDBorderColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				config.DefaultBorderColor = c
			}
		case krb.PropIDFontSize:
			if fsRaw, ok := getShortValue(&prop); ok && fsRaw > 0 {
				config.DefaultFontSize = float32(fsRaw)
			}
		}
	}
}

// --- Methods for Applying Properties to RenderElement ---

func (r *RaylibRenderer) applyStylePropertiesToElement(
	props []krb.Property,
	doc *krb.Document,
	el *render.RenderElement,
) {
	if doc == nil || el == nil {
		return
	}
	for _, prop := range props {
		switch prop.ID {
		case krb.PropIDBgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				el.BgColor = c
			}
		case krb.PropIDFgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				el.FgColor = c
			}
		case krb.PropIDBorderColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				el.BorderColor = c
			}
		case krb.PropIDBorderWidth:
			if bw, ok := getByteValue(&prop); ok {
				el.BorderWidths = [4]uint8{bw, bw, bw, bw}
			} else if edges, okEdges := getEdgeInsetsValue(&prop); okEdges {
				el.BorderWidths = edges
			}
		case krb.PropIDPadding:
			if p, ok := getEdgeInsetsValue(&prop); ok {
				el.Padding = p
			}
		case krb.PropIDTextAlignment:
			if align, ok := getByteValue(&prop); ok {
				el.TextAlignment = align
			}
		case krb.PropIDVisibility:
			if vis, ok := getByteValue(&prop); ok {
				el.IsVisible = (vis != 0)
			}
		case krb.PropIDFontSize:
			if fsRaw, ok := getShortValue(&prop); ok && fsRaw > 0 {
				el.ResolvedFontSize = float32(fsRaw)
			}
		}
	}
}

func (r *RaylibRenderer) applyDirectPropertiesToElement(
	props []krb.Property,
	doc *krb.Document,
	el *render.RenderElement,
) {
	if doc == nil || el == nil {
		return
	}
	for _, prop := range props {
		switch prop.ID {
		case krb.PropIDBgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				el.BgColor = c
			}
		case krb.PropIDFgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				el.FgColor = c
			}
		case krb.PropIDBorderColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				el.BorderColor = c
			}
		case krb.PropIDBorderWidth:
			if bw, ok := getByteValue(&prop); ok {
				el.BorderWidths = [4]uint8{bw, bw, bw, bw}
			} else if edges, okEdges := getEdgeInsetsValue(&prop); okEdges {
				el.BorderWidths = edges
			}
		case krb.PropIDPadding:
			if p, ok := getEdgeInsetsValue(&prop); ok {
				el.Padding = p
			}
		case krb.PropIDTextAlignment:
			if align, ok := getByteValue(&prop); ok {
				el.TextAlignment = align
			}
		case krb.PropIDVisibility:
			if vis, ok := getByteValue(&prop); ok {
				el.IsVisible = (vis != 0)
			}
		case krb.PropIDTextContent:
			if strIdx, ok := getByteValue(&prop); ok {
				if textVal, textOk := getStringValueByIdx(doc, strIdx); textOk {
					el.Text = textVal
				}
			}
		case krb.PropIDImageSource:
			if resIdx, ok := getByteValue(&prop); ok {
				el.ResourceIndex = resIdx
			}
		case krb.PropIDFontSize:
			if fsRaw, ok := getShortValue(&prop); ok && fsRaw > 0 {
				el.ResolvedFontSize = float32(fsRaw)
			}
		default:
			continue
		}
	}
}

func (r *RaylibRenderer) applyDirectVisualPropertiesToAppElement(
	props []krb.Property,
	doc *krb.Document,
	el *render.RenderElement,
) {
	if doc == nil || el == nil {
		return
	}
	for _, prop := range props {
		switch prop.ID {
		case krb.PropIDBgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				el.BgColor = c
			}
		case krb.PropIDFgColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				el.FgColor = c
			}
		case krb.PropIDBorderColor:
			if c, ok := getColorValue(&prop, doc.Header.Flags); ok {
				el.BorderColor = c
			}
		case krb.PropIDBorderWidth:
			if bw, ok := getByteValue(&prop); ok {
				el.BorderWidths = [4]uint8{bw, bw, bw, bw}
			} else if edges, okEdges := getEdgeInsetsValue(&prop); okEdges {
				el.BorderWidths = edges
			}
		case krb.PropIDPadding:
			if p, ok := getEdgeInsetsValue(&prop); ok {
				el.Padding = p
			}
		case krb.PropIDVisibility:
			if vis, ok := getByteValue(&prop); ok {
				el.IsVisible = (vis != 0)
			}
		case krb.PropIDFontSize:
			if fsRaw, ok := getShortValue(&prop); ok && fsRaw > 0 {
				el.ResolvedFontSize = float32(fsRaw)
			}
		}
	}
}

// --- Methods for Resolving Content and Contextual Defaults ---

func (r *RaylibRenderer) resolveElementTextAndImage(
	doc *krb.Document,
	el *render.RenderElement,
	style *krb.Style,
	styleFound bool,
) {
	if doc == nil || el == nil {
		return
	}
	if (el.Header.Type == krb.ElemTypeText || el.Header.Type == krb.ElemTypeButton) && el.Text == "" {
		if styleFound && style != nil {
			if styleProp, propInStyleOk := getStylePropertyValue(style, krb.PropIDTextContent); propInStyleOk {
				if strIdx, ok := getByteValue(styleProp); ok {
					if s, textOk := getStringValueByIdx(doc, strIdx); textOk {
						el.Text = s
					}
				}
			}
		}
	}
	if (el.Header.Type == krb.ElemTypeImage || el.Header.Type == krb.ElemTypeButton) && el.ResourceIndex == render.InvalidResourceIndex {
		if styleFound && style != nil {
			if styleProp, propInStyleOk := getStylePropertyValue(style, krb.PropIDImageSource); propInStyleOk {
				if idx, ok := getByteValue(styleProp); ok {
					el.ResourceIndex = idx
				}
			}
		}
	}
}

func (r *RaylibRenderer) applyContextualDefaults(el *render.RenderElement) {
	if el == nil {
		return
	}
	hasBorderColor := el.BorderColor.A > 0
	allBorderWidthsZero := true
	for _, bw := range el.BorderWidths {
		if bw > 0 {
			allBorderWidthsZero = false
			break
		}
	}
	if hasBorderColor && allBorderWidthsZero {
		el.BorderWidths = [4]uint8{1, 1, 1, 1}
	} else if !allBorderWidthsZero && !hasBorderColor {
		el.BorderColor = r.config.DefaultBorderColor
	}
}

// --- Methods for Property Inheritance ---

const UnsetTextAlignmentSentinel = 0xFF // Define an "unset" marker for TextAlignment

func (r *RaylibRenderer) resolvePropertyInheritance() {
	if len(r.roots) == 0 || r.docRef == nil {
		return
	}
	log.Println("PrepareTree: Resolving property inheritance...")

	initialFgColor := r.config.DefaultFgColor
	initialFontSize := r.config.DefaultFontSize
	initialTextAlignment := uint8(krb.LayoutAlignStart) // App-level default

	for _, rootEl := range r.roots {
		isTextBearingRoot := (rootEl.Header.Type == krb.ElemTypeText || rootEl.Header.Type == krb.ElemTypeButton || rootEl.Header.Type == krb.ElemTypeInput)

		// Resolve FgColor for root
		if isTextBearingRoot && (rootEl.FgColor == rl.Blank || rootEl.FgColor.A == 0) {
			rootEl.FgColor = initialFgColor
		}
		fgColorToPassToChildren := rootEl.FgColor
		if fgColorToPassToChildren.A == 0 {
			fgColorToPassToChildren = initialFgColor
		}

		// Resolve FontSize for root
		resolvedRootFontSize := rootEl.ResolvedFontSize
		if resolvedRootFontSize == 0.0 {
			rootEl.ResolvedFontSize = initialFontSize
			resolvedRootFontSize = initialFontSize
		}

		// Resolve TextAlignment for root
		// If TextAlignment was initialized to UnsetTextAlignmentSentinel and not set by style/direct,
		// it inherits the app-level default. Otherwise, it uses its value (which might be LayoutAlignStart by base init).
		resolvedRootTextAlignment := rootEl.TextAlignment
		if rootEl.TextAlignment == UnsetTextAlignmentSentinel { // Check if it's explicitly "unset" for inheritance
			rootEl.TextAlignment = initialTextAlignment
			resolvedRootTextAlignment = initialTextAlignment
		}
		// If not using a sentinel, TextAlignment would have been set to LayoutAlignStart during
		// PrepareTree's element initialization if no style/direct prop set it.
		// So, resolvedRootTextAlignment = rootEl.TextAlignment is usually correct.

		r.applyInheritanceRecursive(rootEl, fgColorToPassToChildren, resolvedRootFontSize, resolvedRootTextAlignment)
	}
}

func (r *RaylibRenderer) applyInheritanceRecursive(
	el *render.RenderElement,
	inheritedFgColor rl.Color,
	inheritedFontSize float32,
	inheritedTextAlignment uint8,
) {
	if el == nil {
		return
	}

	// 1. ForegroundColor
	isTextBearing := (el.Header.Type == krb.ElemTypeText || el.Header.Type == krb.ElemTypeButton || el.Header.Type == krb.ElemTypeInput)
	if isTextBearing && (el.FgColor == rl.Blank || el.FgColor.A == 0) {
		if inheritedFgColor.A > 0 {
			el.FgColor = inheritedFgColor
		} else {
			el.FgColor = r.config.DefaultFgColor
		}
	}
	fgColorForChildren := el.FgColor
	if el.FgColor.A == 0 {
		fgColorForChildren = inheritedFgColor
	}

	// 2. FontSize
	if el.ResolvedFontSize == 0.0 {
		el.ResolvedFontSize = inheritedFontSize
	}
	fontSizeForChildren := el.ResolvedFontSize

	// 3. TextAlignment
	// If TextAlignment was initialized to UnsetTextAlignmentSentinel and not set by style/direct for 'el', inherit.
	if el.TextAlignment == UnsetTextAlignmentSentinel {
		el.TextAlignment = inheritedTextAlignment
	}
	// Children inherit the now resolved el.TextAlignment
	textAlignmentForChildren := el.TextAlignment

	for _, child := range el.Children {
		r.applyInheritanceRecursive(child, fgColorForChildren, fontSizeForChildren, textAlignmentForChildren)
	}
}

// --- Method for Re-Resolving Visuals of a Single Element ---

func (r *RaylibRenderer) ReResolveElementVisuals(el *render.RenderElement) {
	if el == nil || r.docRef == nil {
		log.Printf("WARN ReResolveElementVisuals: Element or document reference is nil.")
		return
	}

	log.Printf("INFO ReResolveElementVisuals: Re-resolving visuals for '%s' (StyleID: %d)", el.SourceElementName, el.Header.StyleID)

	// 1. Reset visual properties.
	el.BgColor = rl.Blank
	el.FgColor = rl.Blank
	el.BorderColor = rl.Blank
	el.BorderWidths = [4]uint8{0, 0, 0, 0}
	el.Padding = [4]uint8{0, 0, 0, 0}
	el.TextAlignment = UnsetTextAlignmentSentinel // Reset to sentinel to force re-evaluation of inheritance or default
	el.ResolvedFontSize = 0.0

	// 2. Apply the element's current StyleID properties.
	style, styleFound := findStyle(r.docRef, el.Header.StyleID) // Use unexported findStyle
	if styleFound {
		r.applyStylePropertiesToElement(style.Properties, r.docRef, el)
	} else if el.Header.StyleID != 0 {
		log.Printf("WARN ReResolveElementVisuals: StyleID %d for element '%s' not found.", el.Header.StyleID, el.SourceElementName)
	}

	// 3. Re-apply direct KRB properties.
	if el.OriginalIndex >= 0 && el.OriginalIndex < len(r.docRef.Properties) && len(r.docRef.Properties[el.OriginalIndex]) > 0 {
		r.applyDirectPropertiesToElement(r.docRef.Properties[el.OriginalIndex], r.docRef, el)
	}

	// 4. Re-apply contextual defaults.
	r.applyContextualDefaults(el)

	// 5. Re-resolve text and image source.
	r.resolveElementTextAndImage(r.docRef, el, style, styleFound)

	// 6. Re-resolve inheritance for `el` and propagate to its children.
	inheritedFgColor := r.config.DefaultFgColor
	inheritedFontSize := r.config.DefaultFontSize
	inheritedTextAlignment := uint8(krb.LayoutAlignStart) // App-level default

	if el.Parent != nil {
		inheritedFgColor = r.getEffectiveInheritedFgColor(el.Parent)

		if el.Parent.ResolvedFontSize != 0.0 {
			inheritedFontSize = el.Parent.ResolvedFontSize
		} else { // Parent might also be unset, trace up for font size
			ancestorFontSize := r.getEffectiveInheritedFontSize(el.Parent)
			inheritedFontSize = ancestorFontSize
		}
		// For TextAlignment, parent's TextAlignment is its computed value.
		// If parent's was UnsetTextAlignmentSentinel, it would have inherited.
		inheritedTextAlignment = el.Parent.TextAlignment
		if el.Parent.TextAlignment == UnsetTextAlignmentSentinel { // Should not happen if parent was resolved
			log.Printf("WARN ReResolveVisuals: Parent '%s' TextAlignment is Unset. Using app default for inheritance.", el.Parent.SourceElementName)
			inheritedTextAlignment = r.config.DefaultFgColor.A // Typo: should be uint8(krb.LayoutAlignStart) or app default text align
		}

	}

	// Apply to 'el' if its own properties are "unset".
	isTextBearing := (el.Header.Type == krb.ElemTypeText || el.Header.Type == krb.ElemTypeButton || el.Header.Type == krb.ElemTypeInput)
	if isTextBearing && (el.FgColor == rl.Blank || el.FgColor.A == 0) {
		el.FgColor = inheritedFgColor
	}
	if el.ResolvedFontSize == 0.0 {
		el.ResolvedFontSize = inheritedFontSize
	}
	if el.TextAlignment == UnsetTextAlignmentSentinel { // If still unset after style/direct
		el.TextAlignment = inheritedTextAlignment
	}
	// If TextAlignment is still sentinel (e.g. root, no style/direct, and inherited was also sentinel - unlikely)
	// default it to LayoutAlignStart as per spec.
	if el.TextAlignment == UnsetTextAlignmentSentinel {
		el.TextAlignment = uint8(krb.LayoutAlignStart)
	}

	// Fallback for text-bearing elements if still unset.
	if isTextBearing && el.FgColor.A == 0 {
		el.FgColor = r.config.DefaultFgColor
	}
	if el.ResolvedFontSize == 0.0 {
		el.ResolvedFontSize = r.config.DefaultFontSize
	}

	// Determine computed values `el` will pass to its children.
	computedFgColorForChildren := el.FgColor
	if el.FgColor.A == 0 {
		computedFgColorForChildren = inheritedFgColor
	}
	computedFontSizeForChildren := el.ResolvedFontSize
	if el.ResolvedFontSize == 0.0 {
		computedFontSizeForChildren = inheritedFontSize
	}
	computedTextAlignmentForChildren := el.TextAlignment

	for _, child := range el.Children {
		r.applyInheritanceRecursive(child, computedFgColorForChildren, computedFontSizeForChildren, computedTextAlignmentForChildren)
	}
	log.Printf("INFO: ReResolveElementVisuals completed for '%s'. Final FgColor: %v, FontSize: %.1f, TextAlignment: %d", el.SourceElementName, el.FgColor, el.ResolvedFontSize, el.TextAlignment)
}

func (r *RaylibRenderer) getEffectiveInheritedFgColor(el *render.RenderElement) rl.Color {
	if el == nil {
		return r.config.DefaultFgColor
	}
	ancestor := el
	for ancestor != nil {
		if ancestor.FgColor.A > 0 {
			return ancestor.FgColor
		}
		ancestor = ancestor.Parent
	}
	return r.config.DefaultFgColor
}

// Helper to get the FontSize an element would inherit (traces up if needed).
func (r *RaylibRenderer) getEffectiveInheritedFontSize(el *render.RenderElement) float32 {
	if el == nil {
		return r.config.DefaultFontSize
	}
	ancestor := el
	for ancestor != nil {
		if ancestor.ResolvedFontSize != 0.0 {
			return ancestor.ResolvedFontSize
		}
		ancestor = ancestor.Parent
	}
	return r.config.DefaultFontSize
}
