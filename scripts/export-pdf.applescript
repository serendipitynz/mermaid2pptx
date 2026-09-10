-- Batch-export .pptx to PDF through PowerPoint's own rendering engine.
--
--   osascript scripts/export-pdf.applescript sample [more-folders...]
--
-- Structural XML checks cannot see what PowerPoint does at layout time, so the
-- only way to judge a generated deck is to let PowerPoint draw it. This is a
-- manual step: it needs macOS and Microsoft PowerPoint, and nothing in `go
-- test` reaches it.

on run argv
	if (count of argv) is 0 then
		return "usage: osascript scripts/export-pdf.applescript <folder> [<folder>...]"
	end if

	set converted to 0
	set skipped to 0
	set failed to 0
	set report to ""

	repeat with rawFolder in argv
		set folderPath to absolutePath(rawFolder as text)
		set decks to {}
		try
			set folderPath to do shell script "cd " & quoted form of folderPath & " && pwd -P"
			set decks to pptxFilesIn(folderPath)
		on error
			set report to report & "FAIL  " & folderPath & " (not a readable folder)" & linefeed
			set failed to failed + 1
		end try

		repeat with deckPath in decks
			set deckPath to deckPath as text
			set deckName to basename(deckPath)
			-- PowerPoint's lock files are real .pptx on disk but not decks.
			if deckName starts with "~$" then
				set report to report & "SKIP  " & deckName & " (PowerPoint temp file)" & linefeed
				set skipped to skipped + 1
			else
				set pdfPath to (text 1 thru -6 of deckPath) & ".pdf"
				if fileExists(pdfPath) then
					set report to report & "SKIP  " & deckName & " (PDF already exported)" & linefeed
					set skipped to skipped + 1
				else
					try
						exportOne(deckPath, pdfPath)
						set report to report & "OK    " & deckName & " -> " & basename(pdfPath) & linefeed
						set converted to converted + 1
					on error errText
						set report to report & "FAIL  " & deckName & " (" & errText & ")" & linefeed
						set failed to failed + 1
					end try
				end if
			end if
		end repeat
	end repeat

	return report & "converted=" & converted & " skipped=" & skipped & " failed=" & failed
end run

on exportOne(deckPath, pdfPath)
	tell application "Microsoft PowerPoint"
		open POSIX file deckPath
		set p to active presentation
		-- `open` returns nothing, so the deck has to be picked up from the
		-- application state; verify it is the one we asked for before saving
		-- over a PDF named after a different deck. Compare leaf names only:
		-- PowerPoint reports the path through /tmp where `pwd -P` resolves
		-- /private/tmp, and the two never compare equal. A deck that is not
		-- ours is left open — closing it would discard someone's edits.
		if my basename(full name of p) is not my basename(deckPath) then
			error "PowerPoint opened " & (full name of p) & " instead"
		end if
		-- Close the deck even when the save fails; otherwise a folder with one
		-- bad deck leaves PowerPoint holding it open for the rest of the run.
		try
			-- The path must be a `POSIX file`, not a plain string: with a string
			-- PowerPoint reports success and writes nothing.
			save p in POSIX file pdfPath as save as PDF
		on error errText
			close p saving no
			error errText
		end try
		close p saving no
	end tell
end exportOne

on pptxFilesIn(folderPath)
	set listing to do shell script "/usr/bin/find " & quoted form of folderPath & " -maxdepth 1 -type f -name '*.pptx' | /usr/bin/sort"
	if listing is "" then return {}
	set savedDelims to AppleScript's text item delimiters
	-- `do shell script` hands back carriage returns, not linefeeds.
	set AppleScript's text item delimiters to {return, linefeed}
	set paths to text items of listing
	set AppleScript's text item delimiters to savedDelims
	return paths
end pptxFilesIn

on fileExists(posixPath)
	try
		do shell script "/bin/test -e " & quoted form of posixPath
		return true
	on error
		return false
	end try
end fileExists

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
