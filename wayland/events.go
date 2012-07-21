package wayland

import (
	"github.com/sebastianskejoe/gowl"
	"github.com/skelterjohn/go.wde"
)

func handleEvents(w *Window) {
	enterchan := make(chan interface{})
	leavechan := make(chan interface{})
	motionchan := make(chan interface{})
	buttonchan := make(chan interface{})

	w.pointer.AddEnterListener(enterchan)
	w.pointer.AddLeaveListener(leavechan)
	w.pointer.AddMotionListener(motionchan)
	w.pointer.AddButtonListener(buttonchan)

	var lastX, lastY int
	for {
		select {
		case e := <-enterchan:
			enter := e.(gowl.PointerEnter)
			var wee wde.MouseEnteredEvent
			lastX, lastY = int(enter.SurfaceX), int(enter.SurfaceY)
			wee.Where.X = lastX
			wee.Where.Y = lastY
			w.eventchan <- wee
		case _ = <-leavechan:
			w.eventchan <- wde.MouseExitedEvent{}
		case m := <-motionchan:
			motion := m.(gowl.PointerMotion)
			var mme wde.MouseMovedEvent
			lastX, lastY = int(motion.SurfaceX), int(motion.SurfaceY)
			mme.Where.X = lastX
			mme.Where.Y = lastY
			w.eventchan <- mme
		case b := <-buttonchan:
			button := b.(gowl.PointerButton)
			if button.State == 1 {
				var mde wde.MouseDownEvent
				mde.Which = getButton(button.Button)
				mde.Where.X = lastX
				mde.Where.Y = lastY
				w.eventchan <- mde
			} else {
				var mue wde.MouseUpEvent
				mue.Which = getButton(button.Button)
				mue.Where.X = lastX
				mue.Where.Y = lastY
				w.eventchan <- mue
			}
		}
	}
}

func getButton(button uint32) wde.Button {
	switch button {
	case 272:
		return wde.LeftButton
	case 273:
		return wde.MiddleButton
	case 274:
		return wde.RightButton
	}
	return 0
}
