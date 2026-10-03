-- Analyses now keep the file name and MIME type of each attached file, so
-- that apps get the fonts a version's ASS tracks use. Versions analyzed
-- before, with attachments, are analyzed again the next time one is played.
DELETE FROM media_analyses WHERE analysis @? '$.streams[*] ? (@.type == "attachment")';
