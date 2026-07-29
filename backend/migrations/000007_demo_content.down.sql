-- 000008 owns the marker-based reverse operation. Keeping this migration's
-- down inert prevents broad date-based deletion on databases upgraded from v7.
SELECT 1;
