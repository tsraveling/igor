package main

import "time"

func pack(w workPack) {

	time.Sleep(300 * time.Millisecond)
	bins := maxRects(w.files)
	// STUB: Print
}

func maxRects(files []imageFile) []spriteBin {
	/**
	  Initialize:
	  Set F = f(W; H)g.
	  Pack:
	  foreach Rectangle R = (w; h) in the sequence do
			Decide the free rectangle Fi 2 F to pack the rectangle R into.
			If no such rectangle is found, restart with a new bin.
			Decide the orientation for the rectangle and place it at the
			bottom-left of Fi. Denote by B the bounding box of R in the bin
			after it has been positioned.
			Use the MAXRECTS split scheme to subdivide Fi into F 0 and F 00.
			Set F F [ fF 0; F 00g n fFig.
			foreach Free Rectangle F 2 F do
				Compute F n B and subdivide the result into at most four
				new rectangles G1; : : : ; G4.
				Set F F [ fG1; : : : ; G4g n fF g.
			end
			foreach Ordered pair of free rectangles Fi; Fj 2 F do
				if Fi contains Fj then
					Set F F n fFj g
				end
			end
	  end
	*/
}
