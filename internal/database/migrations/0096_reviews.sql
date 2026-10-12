-- The last month and the last year a review was sent for, by their first
-- day in the server's time zone, beside the end of the last week
-- summarized: a restart does not send them again, and a server that was
-- off on the 1st sends them when it starts again. They start at the month
-- and year before the migration's, so that none ended before it is sent.
ALTER TABLE notification_summary
    ALTER COLUMN week_end SET DEFAULT now(),
    ADD COLUMN month_sent date NOT NULL DEFAULT (date_trunc('month', now()) - interval '1 month')::date,
    ADD COLUMN year_sent date NOT NULL DEFAULT (date_trunc('year', now()) - interval '1 year')::date;

-- The year in review is chosen wherever the weekly summary is; the month in
-- review is chosen by no target at first.
UPDATE notification_targets SET events = array_append(events, 'yearly_summary')
WHERE 'weekly_summary' = ANY (events) AND NOT 'yearly_summary' = ANY (events);
