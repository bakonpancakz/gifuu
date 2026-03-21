DO $MAIN$
DECLARE _VERSION INTEGER;
BEGIN
    /*
     * FETCH SCHEMA VERSION
     *  Migration information is stored in the database using this nifty bit of
     *  logic. Please note that there is no rollback procedure so make sure you
     *  test as much as possible on your dev machine, thanks. @_@
     */
	IF NOT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'kvs' AND table_schema = 'public') THEN
		CREATE TABLE kvs (
			key		TEXT NOT NULL UNIQUE,
			value 	TEXT NOT NULL
		);
    END IF;

    SELECT value::INTEGER INTO _VERSION FROM kvs WHERE key = 'gifuu_version';
    IF (SELECT _VERSION IS NULL) THEN
        INSERT INTO kvs VALUES ('gifuu_updated', CURRENT_TIMESTAMP::TEXT);
        INSERT INTO kvs VALUES ('gifuu_version', 0);
        _VERSION := 0;
    END IF;

    /*
     * Version:     1.0.0
     * Name:        Initial Release
     * Description: Initialize Database for Initial Release
     */
	IF (SELECT _VERSION < 1) THEN
        _VERSION := 1;
		RAISE NOTICE 'Upgrading to Version %', _VERSION;

        -- INITIALIZATION --
        CREATE EXTENSION IF NOT EXISTS pg_trgm;

        IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'gifuu_backend') THEN
            DROP OWNED BY gifuu_backend CASCADE;
            DROP ROLE gifuu_backend;
        END IF;

        DROP SCHEMA IF EXISTS gifuu CASCADE;
        CREATE SCHEMA gifuu;

        -- TABLES --
        CREATE TABLE gifuu.upload (
            id                  BIGINT          NOT NULL PRIMARY KEY,                   -- Upload ID
            created             TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,     -- Created At
            upload_address      TEXT            NOT NULL,                               -- (SHA256) IP Address, makes blanket removals possible.
            upload_token        TEXT            NOT NULL,                               -- (SHA256) Access Token, makes write access possible.
            sticker             BOOLEAN         NOT NULL,                               -- Still Image?
            width               INT             NOT NULL,                               -- Calculated Width
            height              INT             NOT NULL,                               -- Calculated Height
            rating              REAL            NOT NULL,                               -- Worst Classification Value
            title               TEXT            NOT NULL                                -- Upload Title
        );

        CREATE TABLE gifuu.tag (
            id                  BIGSERIAL       NOT NULL PRIMARY KEY,                   -- Tag ID
            created             TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,     -- Created At
            label               TEXT            NOT NULL UNIQUE,                        -- Tag Name
            usage               INT             NOT NULL DEFAULT 0                      -- Tag Usage
        );

        CREATE TABLE gifuu.upload_tag (
            gif_id  BIGINT REFERENCES gifuu.upload (id) ON DELETE CASCADE,              -- Relevant GIF ID
            tag_id  BIGINT REFERENCES gifuu.tag    (id) ON DELETE CASCADE,              -- Relevant Tag ID
            PRIMARY KEY(gif_id, tag_id)
        );

        CREATE UNLOGGED TABLE gifuu.session_pow (
            nonce               TEXT            NOT NULL PRIMARY KEY,                   -- PoW Nonce
            expires             TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,     -- PoW Created
            difficulty          INT             NOT NULL                                -- PoW Difficulty
        );

        CREATE UNLOGGED TABLE gifuu.ratelimit_usage (
            subject             TEXT            NOT NULL PRIMARY KEY,                   -- User Identifier
            created             TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,     -- Last Accessed
            usage               INT             NOT NULL                                -- Quota Usage
        );

        CREATE TABLE gifuu.ratelimit_bypass (
            id                  BIGSERIAL       NOT NULL PRIMARY KEY,                   -- Token Identifier
            label               TEXT,                                                   -- Token Label
            token               TEXT            NOT NULL UNIQUE                         -- (SHA256) Token String
        );

        -- INDEXES --
        CREATE INDEX idx_tag_usage_popular  ON gifuu.tag (usage DESC);
        CREATE INDEX gin_tag_metadata       ON gifuu.tag USING GIN (label gin_trgm_ops);
        CREATE INDEX idx_upload_tag_gif     ON gifuu.upload_tag (gif_id);
        CREATE INDEX idx_upload_tag_tag     ON gifuu.upload_tag (tag_id);
        CREATE INDEX idx_upload_created     ON gifuu.upload (created DESC);

        -- TRIGGERS --
        CREATE OR REPLACE FUNCTION gifuu.update_tag_usage() RETURNS TRIGGER AS $$
        BEGIN
            IF TG_OP = 'INSERT' THEN
                UPDATE gifuu.tag SET usage = usage + 1 WHERE id = NEW.tag_id;
            ELSIF TG_OP = 'DELETE' THEN
                UPDATE gifuu.tag SET usage = usage - 1 WHERE id = OLD.tag_id;
            END IF;
            RETURN NULL;
        END;
        $$ LANGUAGE plpgsql;

        CREATE TRIGGER gifuu_tag_usage_insert
        AFTER INSERT ON gifuu.upload_tag
        FOR EACH ROW EXECUTE FUNCTION gifuu.update_tag_usage();

        CREATE TRIGGER gifuu_tag_usage_delete
        AFTER DELETE ON gifuu.upload_tag
        FOR EACH ROW EXECUTE FUNCTION gifuu.update_tag_usage();

        -- USERS --
        CREATE ROLE gifuu_backend LOGIN NOINHERIT;
        GRANT USAGE ON SCHEMA gifuu                 TO gifuu_backend;
        GRANT ALL ON ALL TABLES IN SCHEMA gifuu     TO gifuu_backend;
        GRANT ALL ON ALL SEQUENCES IN SCHEMA gifuu  TO gifuu_backend;

    END IF;

    /*
     * Version:     2.0.0
     * Name:        Moderation Update
     * Description: Update Database for Moderation Features
     */
	IF (SELECT _VERSION < 2) THEN
        _VERSION := 2;
		RAISE NOTICE 'Upgrading to Version %', _VERSION;

        -- Planned Endpoints
	    -- [ ] POST     /animations/{id}/report             // Create report for an upload
	    -- [ ] GET      /admin/dashboard			        // Admin Dashboard
	    -- [ ] GET      /admin/animations/{id}/reports      // Fetch reports for an upload
	    -- [ ] DELETE   /admin/animations/{id}/reports      // Clear reports for an upload
        -- [ ] GET      /admin/animations/{id}/logs		    // Fetch Logs for an upload

        -- TABLES --
        CREATE TABLE gifuu.mod_banned (
            subject             TEXT            NOT NULL PRIMARY KEY,                   -- User Identifier
            created             TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,     -- Created At
            reason              TEXT                                                    -- Given Reason
        );

        CREATE TABLE gifuu.mod_report (
            id                  BIGINT          NOT NULL PRIMARY KEY,                   -- Report ID
            created             TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,     -- Created At
            gifuu_id            BIGINT          NOT NULL,                               -- Relevant GIF ID
            subject             TEXT            NOT NULL,                               -- User Identifier
            reason              INT             NOT NULL,                               -- Report Reason
            FOREIGN KEY (gifuu_id) REFERENCES gifuu.upload (id) ON DELETE CASCADE
        );

        -- USERS --
        GRANT ALL ON ALL TABLES IN SCHEMA gifuu     TO gifuu_backend;
        GRANT ALL ON ALL SEQUENCES IN SCHEMA gifuu  TO gifuu_backend;

    END IF;

    /*
     * HOUSEKEEPING
     *  Uses the "pg_cron" extension to enable automated maintenance without
     *  requiring the use of an external service, see installation guide here:
     *  https://github.com/citusdata/pg_cron#installing-pg_cron
     *
     *  NOTE: This extension is not required while in development but will cause
     *  issues in production as memory usage will climb indefinitely.
     */
    IF NOT EXISTS (SELECT FROM pg_available_extensions WHERE name = 'pg_cron') THEN
        RAISE WARNING 'Extension "pg_cron" is not installed, disabling cron scheduling.';
    ELSE
        CREATE EXTENSION IF NOT EXISTS pg_cron;
        CREATE OR REPLACE PROCEDURE gifuu_reschedule (
            _SCHEDULE 	TEXT,
            _NAME 		TEXT,
            _COMMAND 	TEXT
        )
        LANGUAGE plpgsql SECURITY DEFINER AS $$
            BEGIN
                IF EXISTS (SELECT FROM cron.job WHERE jobname = _NAME) THEN
                    PERFORM cron.unschedule(_NAME);
                END IF;
                PERFORM cron.schedule(_NAME, _SCHEDULE, _COMMAND);
                RAISE NOTICE 'Scheduled "%" (%)', _NAME, _SCHEDULE;
            END;
        $$;
        CALL gifuu_reschedule('0 4 * * *', 'gifuu:clear_ratelimits', $$ TRUNCATE gifuu.ratelimit_usage; $$);
        CALL gifuu_reschedule('0 * * * *', 'gifuu:clear_pow',        $$ DELETE FROM gifuu.session_pow WHERE now() > expires; $$);
    END IF;

    /*
     * UPDATE SCHEMA VERSION
     *  Disabled in development to make iterative changes less annoying. Use the
     *  following query to enter production mode and make changes permanent:
     *
     *  INSERT INTO kvs VALUES ('gifuu_production', 'true');
     */
    IF EXISTS (SELECT FROM kvs WHERE key = 'gifuu_production') THEN
        RAISE NOTICE 'Mode: Production';
	    UPDATE kvs SET value = _VERSION                WHERE key = 'gifuu_version';
	    UPDATE kvs SET value = CURRENT_TIMESTAMP::TEXT WHERE key = 'gifuu_updated';
    ELSE
        RAISE NOTICE 'Mode: Development';
    END IF;

END $MAIN$;
