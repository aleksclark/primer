/** Enum values mirrored from the TV server's schema (see internal/tv/domain). */

/** Media item classes. Only entertainment is rationed by the watch-once ledger. */
export const MEDIA_CLASSES = ["educational", "entertainment", "mixed"];

/** Device kinds: a touch tablet or the leanback TV box. */
export const DEVICE_KINDS = ["tablet", "tv_box"];

/** Schedule day-part blocks for the programmed channel grid. */
export const SCHEDULE_BLOCKS = ["morning", "midday", "afternoon", "evening"];
