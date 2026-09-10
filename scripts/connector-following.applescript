-- Ask PowerPoint whether a connector really follows the shape it is bound to.
--
--   osascript scripts/connector-following.applescript sample/graph1.pptx VALID 40 -30
--
-- The generator writes <a:stCxn>/<a:endCxn> on every edge, but a static PDF or
-- PNG only shows where the lines were drawn -- never whether PowerPoint has
-- actually bound them. Moving a node and re-reading the connectors is the only
-- check that distinguishes a bound connector from a line that happens to touch
-- the node. Manual step: needs macOS and Microsoft PowerPoint.
--
-- Offsets are in points (1 pt = 12700 EMU). The moved deck is written beside
-- the source as <name>-moved.pptx with a <name>-moved.pdf next to it; the
-- source deck is left untouched.

on run argv
	if (count of argv) < 3 then
		return "usage: osascript scripts/connector-following.applescript <deck.pptx> <shape name> <dx pt> [<dy pt>]"
	end if
	set deckPath to absolutePath(item 1 of argv as text)
	if deckPath does not end with ".pptx" then
		error "expected a .pptx deck, got " & deckPath
	end if
	set targetName to item 2 of argv as text
	set dx to (item 3 of argv as text) as real
	if (count of argv) > 3 then
		set dy to (item 4 of argv as text) as real
	else
		set dy to 0
	end if

	set movedPath to (text 1 thru -6 of deckPath) & "-moved.pptx"
	set pdfPath to (text 1 thru -6 of movedPath) & ".pdf"

	tell application "Microsoft PowerPoint"
		open POSIX file deckPath
		set p to active presentation
		if my basename(full name of p) is not my basename(deckPath) then
			error "PowerPoint opened " & (full name of p) & " instead of " & deckPath
		end if
		-- Close the deck even when the work below fails (a missing shape name,
		-- a refused save); otherwise PowerPoint is left holding it open with a
		-- ~$ lock file beside the source.
		try
			set s to slide 1 of p

			set beforeRows to my readConnectors(s)
			set target to my findShape(s, targetName)
			set left position of target to (left position of target) + dx
			set top of target to (top of target) + dy

			save p in POSIX file movedPath as save as Open XML presentation
			save p in POSIX file pdfPath as save as PDF
			set afterRows to my readConnectors(s)
		on error errText
			close p saving no
			error errText
		end try
		close p saving no
	end tell

	return my formatReport(deckPath, targetName, dx, dy, beforeRows, afterRows, movedPath, pdfPath)
end run

-- Each entry is {name, begin shape, end shape, beginX, beginY, endX, endY}.
-- The list is built outside the `tell` block: inside one, `rows` and `end`
-- resolve against PowerPoint's own table vocabulary instead of AppleScript's.
on readConnectors(s)
	set acc to {}
	tell application "Microsoft PowerPoint" to set n to count of shapes of s
	repeat with i from 1 to n
		tell application "Microsoft PowerPoint"
			set sh to shape i of s
			set isConn to is connector of sh
		end tell
		if isConn then
			tell application "Microsoft PowerPoint"
				set cName to name of sh
				set cf to connector format of sh
				if (begin connected of cf) then
					set bName to name of (begin connected shape of cf)
				else
					set bName to "-"
				end if
				if (end connected of cf) then
					set eName to name of (end connected shape of cf)
				else
					set eName to "-"
				end if
			end tell
			set pts to my endpointsOf(sh)
			set end of acc to {cName, bName, eName, item 1 of pts, item 2 of pts, item 3 of pts, item 4 of pts}
		end if
	end repeat
	return acc
end readConnectors

-- Undo flipH/flipV and the rotation about the box centre to recover the two
-- endpoints, mirroring reconstructConnector in internal/convert/convert_test.go
-- so both sides read a connector's geometry by the same rule.
on endpointsOf(sh)
	tell application "Microsoft PowerPoint"
		set ox to left position of sh
		set oy to top of sh
		set cx to width of sh
		set cy to height of sh
		set rot to rotation of sh
		set fh to horizontal flip of sh
		set fv to vertical flip of sh
	end tell
	set midX to cx / 2
	set midY to cy / 2
	set sinDeg to my sinOfDegrees(rot)
	set cosDeg to my sinOfDegrees(rot + 90)
	set pts to {}
	repeat with corner in {{0, 0}, {cx, cy}}
		set px to item 1 of corner
		set py to item 2 of corner
		if fh then set px to cx - px
		if fv then set py to cy - py
		set ddx to px - midX
		set ddy to py - midY
		set end of pts to ox + midX + ddx * cosDeg - ddy * sinDeg
		set end of pts to oy + midY + ddx * sinDeg + ddy * cosDeg
	end repeat
	return pts
end endpointsOf

-- Vanilla AppleScript has no trigonometry (`sin of` needs a scripting
-- addition), so sine is computed here: reduce to the first quadrant, then a
-- Taylor series good to ~1e-8 over it -- far below the tolerance `near` uses.
on sinOfDegrees(deg)
	set d to deg
	repeat while d < 0
		set d to d + 360
	end repeat
	repeat while d >= 360
		set d to d - 360
	end repeat
	set flipSign to 1
	if d > 180 then
		set d to d - 180
		set flipSign to -1
	end if
	if d > 90 then set d to 180 - d
	set x to d * pi / 180
	set x2 to x * x
	return flipSign * x * (1 - x2 / 6 * (1 - x2 / 20 * (1 - x2 / 42 * (1 - x2 / 72 * (1 - x2 / 110)))))
end sinOfDegrees

on findShape(s, targetName)
	tell application "Microsoft PowerPoint"
		repeat with i from 1 to (count of shapes of s)
			set sh to shape i of s
			if (name of sh) is targetName then return sh
		end repeat
	end tell
	error "no shape named " & targetName & " on slide 1"
end findShape

on formatReport(deckPath, targetName, dx, dy, beforeRows, afterRows, movedPath, pdfPath)
	set report to "deck:  " & deckPath & linefeed
	set report to report & "moved: " & targetName & " by (" & dx & ", " & dy & ") pt" & linefeed
	set report to report & "wrote: " & my basename(movedPath) & ", " & my basename(pdfPath) & linefeed & linefeed
	set followed to 0
	set stuck to 0
	set strayed to 0
	set held to 0
	set unbound to 0
	repeat with i from 1 to (count of beforeRows)
		set b to item i of beforeRows
		set a to item i of afterRows
		set report to report & (item 1 of b) & "  [" & (item 2 of b) & " -> " & (item 3 of b) & "]" & linefeed
		set report to report & "    begin " & my endpointLine(b, a, 4, 5)
		set report to report & "    end   " & my endpointLine(b, a, 6, 7)
		repeat with side in {{2, 4, 5}, {3, 6, 7}}
			set sideName to item (item 1 of side) of b
			-- Only edge connectors are supposed to carry stCxn/endCxn; the
			-- "line" connectors (sequence lifelines, compartment dividers) are
			-- free-standing by design, so an unbound end there is not a defect.
			if (sideName is "-") and ((item 1 of b) starts with "edge ") then
				set unbound to unbound + 1
			end if
			set didMove to my endpointMoved(b, a, item 2 of side, item 3 of side)
			if sideName is targetName then
				if didMove then
					set followed to followed + 1
				else
					set stuck to stuck + 1
				end if
			else
				if didMove then
					set strayed to strayed + 1
				else
					set held to held + 1
				end if
			end if
		end repeat
	end repeat
	-- A bound endpoint tracks the shape but is re-routed to whichever
	-- connection site now faces it, so it rarely shifts by exactly (dx, dy);
	-- "moved at all" is what separates a bound connector from a loose line.
	set report to report & linefeed & "endpoints bound to " & targetName & ": followed=" & followed & " stuck=" & stuck & linefeed
	set report to report & "other endpoints: unchanged=" & held & " moved=" & strayed & linefeed
	set report to report & "unbound endpoints on edge connectors: " & unbound & linefeed & linefeed
	return report & "VERDICT: " & my verdictFor(targetName, followed, stuck, unbound)
end formatReport

-- The regression this script exists to catch -- the generator dropping
-- stCxn/endCxn -- leaves nothing bound to the moved node, which as bare counts
-- reads `followed=0 stuck=0` and looks as harmless as a clean run. State the
-- conclusion instead of leaving the reader to infer it from zeroes.
--
-- Any unbound endpoint fails the run, even when the moved node's own endpoints
-- all followed: an unbound endpoint reports its shape as "-", so there is no
-- way to tell whether the binding that went missing was one of the target's.
-- Every `edge ` connector the generator emits is bound at both ends, so there
-- is no legitimate deck this rejects. (U-turn routes fall back to a freeform
-- polyline, which is a <p:sp> rather than a connector -- PowerPoint does not
-- report it as one, so it never reaches this count.)
on verdictFor(targetName, followed, stuck, unbound)
	if unbound > 0 then
		set detail to (unbound as text) & " edge endpoint(s) carry no connection at all; the generator is not emitting stCxn/endCxn for them."
		if (followed + stuck) is 0 then
			return "FAIL - nothing is bound to " & targetName & ", and " & detail & " This run proves nothing about following."
		end if
		return "FAIL - " & detail & " " & (followed as text) & " endpoint(s) bound to " & targetName & " did follow it, but an unbound endpoint names no shape, so one of the missing bindings may be " & targetName & "'s own."
	end if
	if (followed + stuck) is 0 then
		return "FAIL - no connector names " & targetName & " as an endpoint, so this run proves nothing. Check the shape name against the node ids in the .mmd."
	end if
	if stuck > 0 then
		return "FAIL - " & (stuck as text) & " of " & ((followed + stuck) as text) & " endpoints bound to " & targetName & " stayed put; PowerPoint is not treating those as connected."
	end if
	return "OK - all " & (followed as text) & " endpoints bound to " & targetName & " followed it, and every edge endpoint is connected."
end verdictFor

on endpointLine(b, a, xi, yi)
	set x0 to item xi of b
	set y0 to item yi of b
	set x1 to item xi of a
	set y1 to item yi of a
	set txt to "(" & my r2(x0) & ", " & my r2(y0) & ") -> (" & my r2(x1) & ", " & my r2(y1) & ")"
	if my endpointMoved(b, a, xi, yi) then
		return txt & "  shifted by (" & my r2(x1 - x0) & ", " & my r2(y1 - y0) & ")" & linefeed
	end if
	return txt & "  unchanged" & linefeed
end endpointLine

on endpointMoved(b, a, xi, yi)
	return not ((my near((item xi of a) - (item xi of b))) and (my near((item yi of a) - (item yi of b))))
end endpointMoved

-- Geometry round-trips through the EMU PowerPoint stores, so an endpoint that
-- did not move still comes back a hair off; exact equality would report every
-- endpoint as having moved.
on near(v)
	if v < 0 then set v to -v
	return v < 0.05
end near

on r2(v)
	return ((round (v * 100)) / 100)
end r2

on basename(posixPath)
	set savedDelims to AppleScript's text item delimiters
	set AppleScript's text item delimiters to "/"
	set leaf to last text item of posixPath
	set AppleScript's text item delimiters to savedDelims
	return leaf
end basename

-- osascript keeps the calling shell's working directory, so a relative
-- argument can be anchored to it; `POSIX file` itself needs an absolute path.
on absolutePath(p)
	if p starts with "/" then return p
	return (do shell script "pwd -P") & "/" & p
end absolutePath
