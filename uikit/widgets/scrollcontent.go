package widgets

import "github.com/ebitenui/ebitenui/widget"

// scrollContent stashes the content subtree of a ScrollContainer on the
// container's LayoutData so the window chrome can translate it together
// with the viewport: ebitenui keeps the content private
// (ScrollContainer.renderContent re-derives its rect from the viewport),
// and renderContent only re-locates (and relayouts) the content when the
// derived rect differs from the content's current one. A drag that moves
// the viewport AND its content by the same delta therefore computes an
// equal rect and skips its own SetLocation + RequestRelayout — the
// table-body relayout that used to dominate a window drag.
//
// The stash rides on LayoutData, which every layout (grid/row/anchor)
// reads with a comma-ok type assertion, so a non-layout value here
// behaves exactly like the nil it replaces: no layout ever consumed by
// these scroll widgets (the rail's scroll has its own RowLayoutData and
// never registers — it is top chrome, not window content).
type scrollContent struct {
	content widget.PreferredSizeLocateableWidget
}

// AttachScrollContent records content as sc's content subtree. Call it
// once at construction, right after ScrollContainerOpts.Content — the
// content is private to ebitenui, and this is how the window chrome's
// translate walk reaches it.
func AttachScrollContent(sc *widget.ScrollContainer, content widget.PreferredSizeLocateableWidget) {
	sc.GetWidget().LayoutData = scrollContent{content: content}
}

// ScrollContentOf returns the content subtree recorded by
// AttachScrollContent, or nil if the scroll container never registered
// one (the rail's, and any scroll outside a draggable window).
func ScrollContentOf(sc *widget.ScrollContainer) widget.PreferredSizeLocateableWidget {
	if s, ok := sc.GetWidget().LayoutData.(scrollContent); ok {
		return s.content
	}
	return nil
}
