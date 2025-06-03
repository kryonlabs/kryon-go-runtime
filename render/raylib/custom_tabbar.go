// render/raylib/custom_tabbar.go
package raylib

import (
	"fmt"
	"log"
	"strings"

	"github.com/waozixyz/kryon/impl/go/krb"
	"github.com/waozixyz/kryon/impl/go/render"
)

type TabBarHandler struct{}

func (h *TabBarHandler) HandleLayoutAdjustment(
	el *render.RenderElement,
	doc *krb.Document,
	rendererInstance render.Renderer,
) error {
	if el == nil {
		return fmt.Errorf("tabBar handler: received nil element")
	}
	elIDStr := fmt.Sprintf("ElemGlobalIdx %d Name '%s'", el.OriginalIndex, el.SourceElementName)

	if el.Parent == nil {
		log.Printf("WARN TabBarHandler [%s]: cannot adjust layout without a parent.", elIDStr)
		return nil
	}
	if doc == nil {
		return fmt.Errorf("tabBar %s: KRB document is nil", elIDStr)
	}
	if rendererInstance == nil {
		return fmt.Errorf("tabBar %s: renderer instance is nil", elIDStr)
	}

	position, posOk := GetCustomPropertyValue(el, "position", doc)
	if !posOk {
		position = "bottom"
	}
	orientation, orientOk := GetCustomPropertyValue(el, "orientation", doc)
	if !orientOk {
		orientation = "row"
	}

	parent := el.Parent
	parentW, parentH := parent.RenderW, parent.RenderH
	parentX, parentY := parent.RenderX, parent.RenderY
	initialW, initialH := el.RenderW, el.RenderH // These are from PerformLayout on TabBar's container

	log.Printf("DEBUG TabBarHandler [%s]: Adjusting. Pos:'%s' Orient:'%s' | Initial Frame: X:%.1f,Y:%.1f W:%.1fxH:%.1f | Parent Frame: X:%.1f,Y:%.1f W:%.1fxH:%.1f",
		elIDStr, position, orientation, el.RenderX, el.RenderY, initialW, initialH, parentX, parentY, parentW, parentH)

	newX, newY, newW, newH := el.RenderX, el.RenderY, initialW, initialH
	stretchWidth := (strings.ToLower(orientation) == "row")
	stretchHeight := (strings.ToLower(orientation) == "column")

	switch strings.ToLower(position) {
	case "top":
		newY = parentY
		newX = parentX
		if stretchWidth {
			newW = parentW
		}
	case "bottom":
		newY = parentY + parentH - initialH // Use TabBar's own height (initialH) for positioning
		if newY < parentY {
			newY = parentY
		}
		newX = parentX
		if stretchWidth {
			newW = parentW
		}
	case "left":
		newX = parentX
		newY = parentY
		if stretchHeight {
			newH = parentH
		}
	case "right":
		newX = parentX + parentW - initialW // Use TabBar's own width (initialW)
		if newX < parentX {
			newX = parentX
		}
		newY = parentY
		if stretchHeight {
			newH = parentH
		}
	default:
		log.Printf("Warn TabBarHandler [%s]: Unknown position '%s'. Defaulting to 'bottom'.", elIDStr, position)
		position = "bottom"
		newY = parentY + parentH - initialH
		newX = parentX
		if stretchWidth {
			newW = parentW
		}
	}

	// Ensure the TabBar itself does not collapse if its initialH/W was zero before explicit values from style.
	// The TabBar's height (50px) comes from its style 'tab_bar_style_base_row'.
	// This height (initialH) should be used to calculate its Y position from bottom.
	// If initialH was 0 due to some layout issue before handler, this needs care.
	// However, PerformLayout for TabBar container uses its style's height property.
	// So, initialH should be correct (e.g., 50 * scaleFactor).

	finalW := MaxF(1.0, newW)
	finalH := MaxF(1.0, newH)

	el.RenderX, el.RenderY, el.RenderW, el.RenderH = newX, newY, finalW, finalH
	log.Printf("DEBUG TabBarHandler [%s]: Frame adjusted to X:%.1f,Y:%.1f W:%.1fxH:%.1f.", elIDStr, el.RenderX, el.RenderY, el.RenderW, el.RenderH)

	var mainContentSibling *render.RenderElement
	if len(parent.Children) > 1 {
		for _, sibling := range parent.Children {
			if sibling != nil && sibling != el {
				mainContentSibling = sibling
				break
			}
		}
	}

	if mainContentSibling != nil {
		siblingIDStr := fmt.Sprintf("ElemGlobalIdx %d Name '%s'", mainContentSibling.OriginalIndex, mainContentSibling.SourceElementName)
		origSiblingX, origSiblingY, origSiblingW, origSiblingH := mainContentSibling.RenderX, mainContentSibling.RenderY, mainContentSibling.RenderW, mainContentSibling.RenderH

		switch strings.ToLower(position) {
		case "bottom":
			mainContentSibling.RenderH = MaxF(1.0, el.RenderY-mainContentSibling.RenderY)
		case "top":
			newSibY := el.RenderY + el.RenderH
			mainContentSibling.RenderH = MaxF(1.0, (origSiblingY+origSiblingH)-newSibY)
			mainContentSibling.RenderY = newSibY
		case "left":
			newSibX := el.RenderX + el.RenderW
			mainContentSibling.RenderW = MaxF(1.0, (origSiblingX+origSiblingW)-newSibX)
			mainContentSibling.RenderX = newSibX
		case "right":
			mainContentSibling.RenderW = MaxF(1.0, el.RenderX-mainContentSibling.RenderX)
		}
		mainContentSibling.RenderW = MaxF(0, mainContentSibling.RenderW) // Can be 0 if fills space
		mainContentSibling.RenderH = MaxF(0, mainContentSibling.RenderH) // Can be 0
		log.Printf("DEBUG TabBarHandler [%s]: Sibling [%s] adjusted to (X:%.1f,Y:%.1f W:%.1fxH:%.1f)", elIDStr, siblingIDStr, mainContentSibling.RenderX, mainContentSibling.RenderY, mainContentSibling.RenderW, mainContentSibling.RenderH)
	}

	var childLayoutScaleFactor float32 = 1.0
	if appRenderer, ok := rendererInstance.(*RaylibRenderer); ok {
		childLayoutScaleFactor = appRenderer.scaleFactor
	} else {
		log.Printf("WARN TabBarHandler [%s]: Could not get scale factor from renderer instance. Defaulting to 1.0", elIDStr)
	}
	childLayoutScaleFactor = MaxF(1.0, childLayoutScaleFactor)

	log.Printf("DEBUG TabBarHandler [%s]: Relaying out its own children. TabBar Frame (X:%.1f,Y:%.1f W:%.1fxH:%.1f). Scale for children: %.2f",
		elIDStr, el.RenderX, el.RenderY, el.RenderW, el.RenderH, childLayoutScaleFactor)

	if len(el.Children) > 0 {
		elPaddingTop := ScaledF32(el.Padding[0], childLayoutScaleFactor)
		elPaddingRight := ScaledF32(el.Padding[1], childLayoutScaleFactor)
		elPaddingBottom := ScaledF32(el.Padding[2], childLayoutScaleFactor)
		elPaddingLeft := ScaledF32(el.Padding[3], childLayoutScaleFactor)
		elBorderTop := ScaledF32(el.BorderWidths[0], childLayoutScaleFactor)
		elBorderRight := ScaledF32(el.BorderWidths[1], childLayoutScaleFactor)
		elBorderBottom := ScaledF32(el.BorderWidths[2], childLayoutScaleFactor)
		elBorderLeft := ScaledF32(el.BorderWidths[3], childLayoutScaleFactor)

		childrenClientOriginX := el.RenderX + elBorderLeft + elPaddingLeft
		childrenClientOriginY := el.RenderY + elBorderTop + elPaddingTop
		childrenAvailableClientWidth := el.RenderW - (elBorderLeft + elBorderRight + elPaddingLeft + elPaddingRight)
		childrenAvailableClientHeight := el.RenderH - (elBorderTop + elBorderBottom + elPaddingTop + elPaddingBottom)

		childrenAvailableClientWidth = MaxF(0, childrenAvailableClientWidth)
		childrenAvailableClientHeight = MaxF(0, childrenAvailableClientHeight)

		rendererInstance.PerformLayoutChildrenOfElement(
			el,
			childrenClientOriginX,
			childrenClientOriginY,
			childrenAvailableClientWidth,
			childrenAvailableClientHeight,
		)
		// Log bounds of children AFTER re-layout by TabBarHandler
		log.Printf("DEBUG TabBarHandler [%s]: Children bounds after TabBarHandler's re-layout:", elIDStr)
		for _, childButton := range el.Children {
			if childButton != nil {
				log.Printf("  Child '%s': X:%.1f Y:%.1f W:%.1f H:%.1f",
					childButton.SourceElementName, childButton.RenderX, childButton.RenderY, childButton.RenderW, childButton.RenderH)
			}
		}
	}
	return nil
}
