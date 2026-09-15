-- CloudBase PG schema import (from Supabase dump)
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE FUNCTION public.unify_prompt_placeholder(input text) RETURNS text
    LANGUAGE plpgsql
    AS $$
DECLARE
    result TEXT := COALESCE(input, '');

    replacements TEXT[][] := ARRAY[
        -- Go template variables -> simple placeholders
        ['{{.Query}}', '{{query}}'],
        ['{{.Answer}}', '{{answer}}'],
        ['{{.CurrentTime}}', '{{current_time}}'],
        ['{{.CurrentWeek}}', '{{current_week}}'],
        ['{{.Yesterday}}', '{{yesterday}}'],
        ['{{.Contexts}}', '{{contexts}}'],
        -- Go template control structures -> simple placeholders or remove
        ['{{range .Contexts}}', '{{contexts}}'],
        -- Remove Go template syntax
        ['{{if .Contexts}}', ''],
        ['{{else}}', ''],
        ['{{.}}', '']
    ];

    r TEXT[];

BEGIN
    FOREACH r SLICE 1 IN ARRAY replacements LOOP
        result := REPLACE(result, r[1], r[2]);

    END LOOP;

    result := regexp_replace(
        result,
        '\{\{range \.Conversation\}\}[\s\S]*?\{\{end\}\}',
        '{{conversation}}',
        'g'
    );

    result := REPLACE(result, '{{end}}', '');

    RETURN result;

END;

$$;

CREATE FUNCTION public.update_mcp_services_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;

    RETURN NEW;

END;

$$;

CREATE TABLE public.agent_shares (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    agent_id character varying(36) NOT NULL,
    organization_id character varying(36) NOT NULL,
    shared_by_user_id character varying(36) NOT NULL,
    source_tenant_id integer NOT NULL,
    permission character varying(32) DEFAULT 'viewer'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone
);

CREATE TABLE public.audit_logs (
    id bigint NOT NULL,
    tenant_id bigint NOT NULL,
    actor_user_id character varying(36) DEFAULT ''::character varying NOT NULL,
    actor_role character varying(32) DEFAULT ''::character varying NOT NULL,
    action character varying(64) NOT NULL,
    target_type character varying(32) DEFAULT ''::character varying NOT NULL,
    target_id character varying(64) DEFAULT ''::character varying NOT NULL,
    target_user_id character varying(36) DEFAULT ''::character varying NOT NULL,
    request_path character varying(512) DEFAULT ''::character varying NOT NULL,
    request_method character varying(16) DEFAULT ''::character varying NOT NULL,
    outcome character varying(16) DEFAULT 'success'::character varying NOT NULL,
    details jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    scope_type character varying(32) DEFAULT ''::character varying NOT NULL,
    scope_id character varying(64) DEFAULT ''::character varying NOT NULL
);

CREATE SEQUENCE public.audit_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.audit_logs_id_seq OWNED BY public.audit_logs.id;

CREATE TABLE public.auth_tokens (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    user_id character varying(36) NOT NULL,
    token text NOT NULL,
    token_type character varying(50) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    is_revoked boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.billing_record_items (
    id character varying(36) NOT NULL,
    record_id character varying(36) NOT NULL,
    kind character varying(20) NOT NULL,
    name character varying(128) NOT NULL,
    period character varying(20) DEFAULT ''::character varying NOT NULL,
    qty numeric(18,2) DEFAULT 0 NOT NULL,
    rate numeric(18,6) DEFAULT 0 NOT NULL,
    fee numeric(18,2) DEFAULT 0 NOT NULL,
    sort integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.billing_records (
    id character varying(36) NOT NULL,
    billing_tenant_id character varying(36) NOT NULL,
    month character varying(7) NOT NULL,
    status character varying(20) DEFAULT 'generated'::character varying NOT NULL,
    total_kwh numeric(18,2) DEFAULT 0 NOT NULL,
    line_loss numeric(18,2) DEFAULT 0 NOT NULL,
    ratio numeric(18,6) DEFAULT 0 NOT NULL,
    bill_total_amount numeric(18,2) DEFAULT 0 NOT NULL,
    dorm_kwh numeric(18,2) DEFAULT 0 NOT NULL,
    dorm_fee numeric(18,2) DEFAULT 0 NOT NULL,
    water_usage numeric(18,2) DEFAULT 0 NOT NULL,
    water_fee numeric(18,2) DEFAULT 0 NOT NULL,
    industrial_fee numeric(18,2) DEFAULT 0 NOT NULL,
    total_fee numeric(18,2) DEFAULT 0 NOT NULL,
    remark text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE public.billing_tenant_items (
    id character varying(36) NOT NULL,
    billing_tenant_id character varying(36) NOT NULL,
    item_key character varying(64) NOT NULL,
    item_name character varying(128) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    sort integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    category character varying(255) DEFAULT ''::character varying NOT NULL
);

CREATE TABLE public.billing_tenant_meter_refs (
    id character varying(36) NOT NULL,
    billing_tenant_id character varying(36) NOT NULL,
    category character varying(20) NOT NULL,
    meter_id character varying(36) NOT NULL,
    meter_name character varying(128) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.billing_tenant_settings (
    id character varying(36) NOT NULL,
    billing_tenant_id character varying(36) NOT NULL,
    dorm_price numeric(18,4) DEFAULT 1 NOT NULL,
    water_price numeric(18,4) DEFAULT 5.22 NOT NULL,
    bill_kb_id character varying(36) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.billing_tenants (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    name character varying(128) NOT NULL,
    remark text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    tenant_no character varying(64) DEFAULT ''::character varying NOT NULL,
    allocation_mode character varying(32) DEFAULT '按比例分摊'::character varying NOT NULL,
    lease_start date,
    lease_years integer DEFAULT 0 NOT NULL,
    contact character varying(64) DEFAULT ''::character varying NOT NULL,
    phone character varying(32) DEFAULT ''::character varying NOT NULL
);

CREATE TABLE public.billing_time_meter_readings (
    id character varying(36) NOT NULL,
    meter_id character varying(36) NOT NULL,
    month character varying(7) NOT NULL,
    deep_prev numeric(18,2) DEFAULT 0 NOT NULL,
    deep_curr numeric(18,2) DEFAULT 0 NOT NULL,
    peak_prev numeric(18,2) DEFAULT 0 NOT NULL,
    peak_curr numeric(18,2) DEFAULT 0 NOT NULL,
    flat_prev numeric(18,2) DEFAULT 0 NOT NULL,
    flat_curr numeric(18,2) DEFAULT 0 NOT NULL,
    valley_prev numeric(18,2) DEFAULT 0 NOT NULL,
    valley_curr numeric(18,2) DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.billing_time_meters (
    id character varying(36) NOT NULL,
    billing_tenant_id character varying(36) NOT NULL,
    meter_type character varying(20) NOT NULL,
    name character varying(128) NOT NULL,
    rate numeric(12,4) DEFAULT 1 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    meter_no character varying(64) DEFAULT ''::character varying NOT NULL,
    meter_kind character varying(16) DEFAULT 'time'::character varying NOT NULL,
    owner_unit character varying(128) DEFAULT ''::character varying NOT NULL,
    use_unit character varying(128) DEFAULT ''::character varying NOT NULL,
    manager character varying(64) DEFAULT ''::character varying NOT NULL,
    contact character varying(64) DEFAULT ''::character varying NOT NULL,
    meter_mode character varying(16) DEFAULT 'manual'::character varying NOT NULL,
    install_date date,
    remark text
);

CREATE TABLE public.billing_water_meter_readings (
    id character varying(64) NOT NULL,
    meter_id character varying(64) NOT NULL,
    month character varying(16) NOT NULL,
    prev numeric(18,2) DEFAULT 0 NOT NULL,
    curr numeric(18,2) DEFAULT 0 NOT NULL,
    price numeric(18,4) DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.billing_water_meters (
    id character varying(64) NOT NULL,
    billing_tenant_id character varying(64) DEFAULT ''::character varying NOT NULL,
    name character varying(128) DEFAULT ''::character varying NOT NULL,
    meter_no character varying(64) DEFAULT ''::character varying NOT NULL,
    meter_kind character varying(16) DEFAULT 'total'::character varying NOT NULL,
    owner_unit character varying(128) DEFAULT ''::character varying NOT NULL,
    use_unit character varying(128) DEFAULT ''::character varying NOT NULL,
    manager character varying(64) DEFAULT ''::character varying NOT NULL,
    contact character varying(64) DEFAULT ''::character varying NOT NULL,
    meter_mode character varying(16) DEFAULT 'manual'::character varying NOT NULL,
    install_date date,
    remark text,
    rate numeric(12,4) DEFAULT 1 NOT NULL,
    price numeric(18,4) DEFAULT 0 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone
);

CREATE TABLE public.chunk_revisions (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    knowledge_id character varying(36) NOT NULL,
    chunk_id character varying(36) NOT NULL,
    revision integer NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    editor_id character varying(64) DEFAULT ''::character varying NOT NULL,
    edit_source character varying(16) DEFAULT 'user'::character varying NOT NULL,
    edited_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE public.chunks_seq_id_seq
    START WITH 100000000
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE TABLE public.chunks (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    knowledge_id character varying(36) NOT NULL,
    content text NOT NULL,
    chunk_index integer NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    start_at integer NOT NULL,
    end_at integer NOT NULL,
    pre_chunk_id character varying(36),
    next_chunk_id character varying(36),
    chunk_type character varying(20) DEFAULT 'text'::character varying NOT NULL,
    parent_chunk_id character varying(36),
    image_info text,
    relation_chunks jsonb,
    indirect_relation_chunks jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    metadata jsonb,
    tag_id character varying(36),
    status integer DEFAULT 0 NOT NULL,
    content_hash character varying(64),
    flags integer DEFAULT 1 NOT NULL,
    seq_id bigint DEFAULT nextval('public.chunks_seq_id_seq'::regclass) NOT NULL,
    video_info text,
    source_content text DEFAULT ''::text NOT NULL,
    content_revision integer DEFAULT 0 NOT NULL,
    index_status character varying(16) DEFAULT 'ready'::character varying NOT NULL,
    last_editor_id character varying(64) DEFAULT ''::character varying NOT NULL,
    context_header text DEFAULT ''::text NOT NULL
);

CREATE TABLE public.custom_agents (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    avatar character varying(64),
    is_builtin boolean DEFAULT false NOT NULL,
    tenant_id integer NOT NULL,
    created_by character varying(36),
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    runnable_by_viewer boolean DEFAULT true NOT NULL
);

CREATE TABLE public.data_sources (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    name character varying(255) NOT NULL,
    type character varying(50) NOT NULL,
    config jsonb,
    sync_schedule character varying(100),
    sync_mode character varying(20) DEFAULT 'incremental'::character varying,
    status character varying(32) DEFAULT 'active'::character varying,
    conflict_strategy character varying(32) DEFAULT 'overwrite'::character varying,
    sync_deletions boolean DEFAULT true,
    last_sync_at timestamp without time zone,
    last_sync_cursor jsonb,
    last_sync_result jsonb,
    error_message text,
    sync_log_retention_days integer DEFAULT 30,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);

CREATE TABLE public.embed_channels (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id bigint NOT NULL,
    agent_id character varying(36) DEFAULT 'builtin-quick-answer'::character varying NOT NULL,
    name character varying(255) DEFAULT ''::character varying NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    publish_token character varying(64) DEFAULT ''::character varying NOT NULL,
    allowed_origins jsonb DEFAULT '[]'::jsonb NOT NULL,
    welcome_message text DEFAULT ''::text NOT NULL,
    rate_limit_per_minute integer DEFAULT 30 NOT NULL,
    rate_limit_per_day integer DEFAULT 10000 NOT NULL,
    primary_color character varying(32) DEFAULT ''::character varying NOT NULL,
    page_title character varying(255) DEFAULT ''::character varying NOT NULL,
    header_title_mode character varying(32) DEFAULT 'channel'::character varying NOT NULL,
    show_suggested_questions boolean DEFAULT true NOT NULL,
    widget_position character varying(32) DEFAULT 'bottom-right'::character varying NOT NULL,
    allow_web_search boolean DEFAULT false NOT NULL,
    allow_memory boolean DEFAULT false NOT NULL,
    allow_file_upload boolean DEFAULT false NOT NULL,
    default_locale character varying(16) DEFAULT ''::character varying NOT NULL,
    webhook_url character varying(512) DEFAULT ''::character varying NOT NULL,
    webhook_secret character varying(128) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE public.embeddings (
    id integer NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    source_id character varying(64) NOT NULL,
    source_type integer NOT NULL,
    chunk_id character varying(64),
    knowledge_id character varying(64),
    knowledge_base_id character varying(64),
    content text,
    dimension integer NOT NULL,
    embedding halfvec,
    is_enabled boolean DEFAULT true,
    tag_id character varying(36)
);

CREATE SEQUENCE public.embeddings_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.embeddings_id_seq OWNED BY public.embeddings.id;

CREATE TABLE public.im_channel_sessions (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    platform character varying(20) NOT NULL,
    user_id character varying(128) NOT NULL,
    chat_id character varying(128) DEFAULT ''::character varying NOT NULL,
    session_id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    agent_id character varying(36) DEFAULT ''::character varying,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone,
    im_channel_id character varying(36) DEFAULT ''::character varying,
    thread_id character varying(128) DEFAULT ''::character varying NOT NULL
);

CREATE TABLE public.im_channels (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id bigint NOT NULL,
    agent_id character varying(36) NOT NULL,
    platform character varying(20) NOT NULL,
    name character varying(255) DEFAULT ''::character varying NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    mode character varying(20) DEFAULT 'websocket'::character varying NOT NULL,
    output_mode character varying(20) DEFAULT 'stream'::character varying NOT NULL,
    credentials jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone,
    knowledge_base_id character varying(36) DEFAULT ''::character varying,
    bot_identity character varying(255) DEFAULT ''::character varying NOT NULL,
    session_mode character varying(20) DEFAULT 'user'::character varying NOT NULL,
    CONSTRAINT chk_im_channels_session_mode CHECK (((session_mode)::text = ANY ((ARRAY['user'::character varying, 'thread'::character varying])::text[])))
);

CREATE TABLE public.kb_shares (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    organization_id character varying(36) NOT NULL,
    shared_by_user_id character varying(36) NOT NULL,
    source_tenant_id integer NOT NULL,
    permission character varying(32) DEFAULT 'viewer'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone
);

CREATE TABLE public.knowledge_bases (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    tenant_id integer NOT NULL,
    chunking_config jsonb DEFAULT '{"chunk_size": 512, "chunk_overlap": 50, "split_markers": ["\n\n", "\n", "。"], "keep_separator": true}'::jsonb NOT NULL,
    image_processing_config jsonb DEFAULT '{"model_id": "", "enable_multimodal": false}'::jsonb NOT NULL,
    embedding_model_id character varying(64) NOT NULL,
    summary_model_id character varying(64) NOT NULL,
    cos_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    vlm_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    extract_config jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    is_temporary boolean DEFAULT false NOT NULL,
    type character varying(32) DEFAULT 'document'::character varying NOT NULL,
    faq_config jsonb,
    question_generation_config jsonb,
    storage_provider_config jsonb,
    is_pinned boolean DEFAULT false NOT NULL,
    pinned_at timestamp with time zone,
    asr_config jsonb,
    vector_store_id character varying(36),
    wiki_config jsonb,
    indexing_strategy jsonb,
    creator_id character varying(36),
    storage_backend_id character varying(36),
    auto_tag_config jsonb,
    recognition_config json
);

CREATE TABLE public.knowledge_processing_spans (
    id bigint NOT NULL,
    knowledge_id character varying(64) NOT NULL,
    attempt integer DEFAULT 1 NOT NULL,
    span_id character varying(64) NOT NULL,
    parent_span_id character varying(64),
    name character varying(255) NOT NULL,
    kind character varying(16) NOT NULL,
    status character varying(16) NOT NULL,
    input jsonb,
    output jsonb,
    metadata jsonb,
    error_code character varying(64),
    error_message text,
    error_detail text,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    duration_ms bigint,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE SEQUENCE public.knowledge_processing_spans_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.knowledge_processing_spans_id_seq OWNED BY public.knowledge_processing_spans.id;

CREATE TABLE public.knowledge_tag_relations (
    knowledge_id character varying(36) NOT NULL,
    tag_id character varying(36) NOT NULL,
    created_at timestamp with time zone DEFAULT now()
);

CREATE SEQUENCE public.knowledge_tags_seq_id_seq
    START WITH 10000000
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE TABLE public.knowledge_tags (
    id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    name character varying(128) NOT NULL,
    color character varying(32),
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    seq_id bigint DEFAULT nextval('public.knowledge_tags_seq_id_seq'::regclass) NOT NULL
);

CREATE TABLE public.knowledges (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    type character varying(50) NOT NULL,
    title character varying(255) NOT NULL,
    description text,
    source character varying(2048) NOT NULL,
    parse_status character varying(50) DEFAULT 'unprocessed'::character varying NOT NULL,
    enable_status character varying(50) DEFAULT 'enabled'::character varying NOT NULL,
    embedding_model_id character varying(64),
    file_name character varying(255),
    file_type character varying(50),
    file_size bigint,
    file_path text,
    file_hash character varying(64),
    storage_size bigint DEFAULT 0 NOT NULL,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    processed_at timestamp with time zone,
    error_message text,
    deleted_at timestamp with time zone,
    summary_status character varying(32) DEFAULT 'none'::character varying,
    last_faq_import_result json,
    channel character varying(50) DEFAULT 'web'::character varying NOT NULL,
    pending_subtasks_count integer DEFAULT 0 NOT NULL,
    custom_metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    folder_path character varying(1024) DEFAULT ''::character varying NOT NULL
);

CREATE TABLE public.mcp_oauth_clients (
    id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    service_id character varying(36) NOT NULL,
    client_id character varying(512) NOT NULL,
    client_secret text,
    redirect_uri character varying(1024),
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE public.mcp_oauth_tokens (
    id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    user_id character varying(512) NOT NULL,
    service_id character varying(36) NOT NULL,
    access_token text,
    refresh_token text,
    token_type character varying(32),
    expires_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    principal_type character varying(32) NOT NULL,
    principal_id character varying(512) NOT NULL,
    refresh_lease_id character varying(36),
    refresh_lease_until timestamp with time zone
);

CREATE TABLE public.mcp_services (
    id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    enabled boolean DEFAULT true,
    transport_type character varying(50) NOT NULL,
    url character varying(512),
    headers jsonb,
    auth_config jsonb,
    advanced_config jsonb,
    stdio_config jsonb,
    env_vars jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone,
    is_builtin boolean DEFAULT false NOT NULL
);

CREATE TABLE public.mcp_tool_approvals (
    id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    service_id character varying(36) NOT NULL,
    tool_name character varying(512) NOT NULL,
    require_approval boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE public.memory_doc_affinity (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    subject_id character varying(512) NOT NULL,
    knowledge_id character varying(36) NOT NULL,
    knowledge_base_id character varying(36) DEFAULT ''::character varying NOT NULL,
    title character varying(512) DEFAULT ''::character varying NOT NULL,
    hits integer DEFAULT 0 NOT NULL,
    last_used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.memory_item_embeddings (
    item_id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    subject_id character varying(512) NOT NULL,
    model_id character varying(64) DEFAULT ''::character varying NOT NULL,
    dims integer DEFAULT 0 NOT NULL,
    vector bytea,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.memory_items (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    subject_id character varying(512) NOT NULL,
    kind character varying(32) NOT NULL,
    content text NOT NULL,
    topic character varying(255) DEFAULT ''::character varying NOT NULL,
    normalized_key character varying(255) DEFAULT ''::character varying NOT NULL,
    importance smallint DEFAULT 3 NOT NULL,
    origin character varying(16) DEFAULT 'extracted'::character varying NOT NULL,
    status character varying(16) DEFAULT 'active'::character varying NOT NULL,
    source_session_id character varying(36),
    source_message_id character varying(36),
    valid_from timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    invalid_at timestamp with time zone,
    expires_at timestamp with time zone,
    superseded_by character varying(36),
    last_used_at timestamp with time zone,
    use_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.memory_subjects (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    subject_id character varying(512) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    block_text text DEFAULT ''::text NOT NULL,
    block_updated_at timestamp with time zone,
    item_count integer DEFAULT 0 NOT NULL,
    last_extracted_at timestamp with time zone,
    extract_cursor timestamp with time zone,
    pending_sessions jsonb,
    extract_scheduled_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    consolidated_at timestamp with time zone,
    forced_consolidated_at timestamp with time zone
);

CREATE TABLE public.memory_tombstones (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    subject_id character varying(512) NOT NULL,
    topic character varying(255) DEFAULT ''::character varying NOT NULL,
    fingerprint character varying(64) NOT NULL,
    source_message_id character varying(36),
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.memory_topic_stats (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    subject_id character varying(512) NOT NULL,
    normalized_key character varying(255) NOT NULL,
    topic character varying(255) DEFAULT ''::character varying NOT NULL,
    hits integer DEFAULT 0 NOT NULL,
    last_seen_at timestamp with time zone,
    promoted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    aliases jsonb DEFAULT '[]'::jsonb NOT NULL
);

CREATE TABLE public.message_suggestion_events (
    id bigint NOT NULL,
    tenant_id integer NOT NULL,
    session_id character varying(36) NOT NULL,
    suggestion_set_id character varying(36) NOT NULL,
    question_id character varying(64) DEFAULT ''::character varying NOT NULL,
    event_type character varying(32) NOT NULL,
    actor_id character varying(512) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE SEQUENCE public.message_suggestion_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.message_suggestion_events_id_seq OWNED BY public.message_suggestion_events.id;

CREATE TABLE public.message_suggestion_sets (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    session_id character varying(36) NOT NULL,
    assistant_message_id character varying(36) NOT NULL,
    agent_id character varying(36) DEFAULT ''::character varying NOT NULL,
    agent_tenant_id integer DEFAULT 0 NOT NULL,
    placement character varying(32) NOT NULL,
    config_hash character varying(64) NOT NULL,
    locale character varying(16) DEFAULT ''::character varying NOT NULL,
    status character varying(16) NOT NULL,
    allow_regenerate boolean DEFAULT false NOT NULL,
    suppression_reason character varying(64) DEFAULT ''::character varying NOT NULL,
    questions jsonb DEFAULT '[]'::jsonb NOT NULL,
    model_id character varying(64) DEFAULT ''::character varying NOT NULL,
    prompt_tokens integer DEFAULT 0 NOT NULL,
    completion_tokens integer DEFAULT 0 NOT NULL,
    latency_ms bigint DEFAULT 0 NOT NULL,
    error_code character varying(64) DEFAULT ''::character varying NOT NULL,
    lease_until timestamp with time zone,
    generated_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE public.messages (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    request_id character varying(36) NOT NULL,
    session_id character varying(36) NOT NULL,
    role character varying(50) NOT NULL,
    content text NOT NULL,
    knowledge_references jsonb DEFAULT '[]'::jsonb NOT NULL,
    agent_steps jsonb,
    is_completed boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    mentioned_items jsonb DEFAULT '[]'::jsonb,
    is_fallback boolean DEFAULT false,
    agent_duration_ms bigint DEFAULT 0,
    knowledge_id character varying(36),
    images jsonb DEFAULT '[]'::jsonb,
    channel character varying(50) DEFAULT ''::character varying NOT NULL,
    rendered_content text DEFAULT ''::text NOT NULL,
    attachments jsonb DEFAULT '[]'::jsonb,
    agent_id character varying(36) DEFAULT ''::character varying NOT NULL,
    agent_tenant_id integer DEFAULT 0 NOT NULL,
    model_id character varying(64) DEFAULT ''::character varying NOT NULL,
    execution_context jsonb DEFAULT '{}'::jsonb NOT NULL,
    artifacts jsonb DEFAULT '[]'::jsonb,
    used_memories jsonb,
    usage jsonb
);

CREATE TABLE public.models (
    id character varying(64) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    name character varying(255) NOT NULL,
    display_name character varying(255) DEFAULT ''::character varying NOT NULL,
    type character varying(50) NOT NULL,
    source character varying(50) NOT NULL,
    description text,
    parameters jsonb NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    status character varying(50) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    is_builtin boolean DEFAULT false NOT NULL,
    managed_by character varying(32) DEFAULT ''::character varying NOT NULL
);

CREATE TABLE public.organization_join_requests (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    organization_id character varying(36) NOT NULL,
    user_id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    requested_role character varying(32) DEFAULT 'viewer'::character varying NOT NULL,
    request_type character varying(32) DEFAULT 'join'::character varying NOT NULL,
    prev_role character varying(32),
    message text,
    reviewed_by character varying(36),
    reviewed_at timestamp with time zone,
    review_message text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.organization_members_pre_plan3 (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    organization_id character varying(36) NOT NULL,
    user_id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    role character varying(32) DEFAULT 'viewer'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.organization_tenant_members (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    organization_id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    role character varying(32) DEFAULT 'viewer'::character varying NOT NULL,
    representative_user_id character varying(36) DEFAULT ''::character varying NOT NULL,
    joined_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE public.organizations (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    owner_id character varying(36) NOT NULL,
    invite_code character varying(32),
    require_approval boolean DEFAULT false,
    invite_code_expires_at timestamp with time zone,
    invite_code_validity_days smallint DEFAULT 7 NOT NULL,
    avatar character varying(512) DEFAULT ''::character varying,
    searchable boolean DEFAULT false NOT NULL,
    member_limit integer DEFAULT 50 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    owner_tenant_id bigint NOT NULL
);

CREATE TABLE public.resource_access_grants (
    id character varying(36) NOT NULL,
    token_hash character varying(64) NOT NULL,
    resource_id character varying(36) NOT NULL,
    access_scope character varying(16) DEFAULT 'read'::character varying NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.resource_bindings (
    id character varying(36) NOT NULL,
    resource_id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    owner_type character varying(32) NOT NULL,
    owner_id character varying(64) NOT NULL,
    relation character varying(32) DEFAULT 'attachment'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.resources (
    id character varying(36) NOT NULL,
    handle character varying(22) NOT NULL,
    tenant_id bigint NOT NULL,
    storage_backend_id character varying(36),
    provider character varying(32) NOT NULL,
    physical_path text NOT NULL,
    location_hash character varying(64) NOT NULL,
    kind character varying(32) DEFAULT 'file'::character varying NOT NULL,
    mime_type character varying(255) DEFAULT ''::character varying NOT NULL,
    original_name character varying(1024) DEFAULT ''::character varying NOT NULL,
    size bigint DEFAULT 0 NOT NULL,
    content_hash character varying(64) DEFAULT ''::character varying NOT NULL,
    lifecycle character varying(16) DEFAULT 'persistent'::character varying NOT NULL,
    expires_at timestamp without time zone,
    state character varying(16) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);

CREATE TABLE public.schema_migrations (
    version bigint NOT NULL,
    dirty boolean NOT NULL
);

CREATE TABLE public.sessions (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    tenant_id integer NOT NULL,
    title character varying(255),
    description text,
    knowledge_base_id character varying(36),
    max_rounds integer DEFAULT 5 NOT NULL,
    enable_rewrite boolean DEFAULT true NOT NULL,
    fallback_strategy character varying(255) DEFAULT 'fixed'::character varying NOT NULL,
    fallback_response text DEFAULT '很抱歉，我暂时无法回答这个问题。'::text NOT NULL,
    keyword_threshold double precision DEFAULT 0.5 NOT NULL,
    vector_threshold double precision DEFAULT 0.5 NOT NULL,
    rerank_model_id character varying(64),
    embedding_top_k integer DEFAULT 10 NOT NULL,
    rerank_top_k integer DEFAULT 10 NOT NULL,
    rerank_threshold double precision DEFAULT 0.65 NOT NULL,
    summary_model_id character varying(64),
    summary_parameters jsonb DEFAULT '{}'::jsonb NOT NULL,
    agent_config jsonb,
    context_config jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    agent_id character varying(36),
    user_id character varying(512),
    is_pinned boolean DEFAULT false NOT NULL,
    pinned_at timestamp with time zone,
    sandbox_config_id character varying(36) DEFAULT NULL::character varying
);

CREATE TABLE public.storage_backends (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    provider character varying(32) NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    source character varying(16) DEFAULT 'user'::character varying NOT NULL,
    status character varying(16) DEFAULT 'active'::character varying NOT NULL,
    legacy_alias boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);

CREATE TABLE public.sync_logs (
    id character varying(36) NOT NULL,
    data_source_id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    status character varying(32) NOT NULL,
    started_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    finished_at timestamp without time zone,
    items_total integer DEFAULT 0,
    items_created integer DEFAULT 0,
    items_updated integer DEFAULT 0,
    items_deleted integer DEFAULT 0,
    items_skipped integer DEFAULT 0,
    items_failed integer DEFAULT 0,
    error_message text,
    result jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.system_settings (
    id bigint NOT NULL,
    key character varying(128) NOT NULL,
    value jsonb NOT NULL,
    value_type character varying(16) NOT NULL,
    category character varying(32) NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_secret boolean DEFAULT false NOT NULL,
    requires_restart boolean DEFAULT false NOT NULL,
    last_modified_by character varying(36) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE SEQUENCE public.system_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.system_settings_id_seq OWNED BY public.system_settings.id;

CREATE TABLE public.task_dead_letters (
    id bigint NOT NULL,
    tenant_id bigint NOT NULL,
    task_type character varying(64) NOT NULL,
    scope character varying(32) NOT NULL,
    scope_id character varying(64) NOT NULL,
    related_id character varying(64) DEFAULT ''::character varying NOT NULL,
    payload jsonb NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    fail_count integer NOT NULL,
    failed_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE public.task_dead_letters_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.task_dead_letters_id_seq OWNED BY public.task_dead_letters.id;

CREATE TABLE public.task_pending_ops (
    id bigint NOT NULL,
    tenant_id bigint NOT NULL,
    task_type character varying(64) NOT NULL,
    scope character varying(32) NOT NULL,
    scope_id character varying(64) NOT NULL,
    op character varying(32) NOT NULL,
    dedup_key character varying(128) DEFAULT ''::character varying NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    fail_count integer DEFAULT 0 NOT NULL,
    enqueued_at timestamp with time zone DEFAULT now() NOT NULL,
    claimed_at timestamp with time zone
);

CREATE SEQUENCE public.task_pending_ops_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.task_pending_ops_id_seq OWNED BY public.task_pending_ops.id;

CREATE TABLE public.temporary_documents (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    session_id character varying(36) NOT NULL,
    resource_ref text NOT NULL,
    file_name character varying(1024) NOT NULL,
    file_type character varying(32) NOT NULL,
    mime_type character varying(255) DEFAULT ''::character varying NOT NULL,
    file_size bigint NOT NULL,
    status character varying(16) DEFAULT 'uploaded'::character varying NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    chunks jsonb DEFAULT '[]'::jsonb NOT NULL,
    image_refs jsonb DEFAULT '[]'::jsonb NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    processing_options jsonb DEFAULT '{}'::jsonb NOT NULL,
    token_count integer DEFAULT 0 NOT NULL,
    chunk_count integer DEFAULT 0 NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    expires_at timestamp without time zone NOT NULL,
    started_at timestamp without time zone,
    ready_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);

CREATE TABLE public.tenant_api_keys (
    id bigint NOT NULL,
    tenant_id integer,
    name character varying(128) NOT NULL,
    key_hash character varying(64) NOT NULL,
    api_key text DEFAULT ''::text NOT NULL,
    full_access boolean DEFAULT false NOT NULL,
    knowledge_base_ids jsonb DEFAULT '[]'::jsonb NOT NULL,
    capabilities jsonb DEFAULT '[]'::jsonb NOT NULL,
    last_used_at timestamp with time zone,
    expires_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    scope_type character varying(16) DEFAULT 'tenant'::character varying NOT NULL,
    CONSTRAINT chk_tenant_api_keys_scope CHECK (((((scope_type)::text = 'tenant'::text) AND (tenant_id IS NOT NULL)) OR (((scope_type)::text = 'platform'::text) AND (tenant_id IS NULL) AND (full_access = false))))
);

CREATE SEQUENCE public.tenant_api_keys_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.tenant_api_keys_id_seq OWNED BY public.tenant_api_keys.id;

CREATE TABLE public.tenant_disabled_shared_agents (
    tenant_id bigint NOT NULL,
    agent_id character varying(36) NOT NULL,
    source_tenant_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.tenant_invitations (
    id bigint NOT NULL,
    tenant_id integer NOT NULL,
    invitee_user_id character varying(36) DEFAULT ''::character varying NOT NULL,
    invited_by character varying(36),
    role character varying(20) NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    message character varying(500),
    expires_at timestamp with time zone NOT NULL,
    responded_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone,
    token character varying(64) DEFAULT ''::character varying NOT NULL,
    accepted_count integer DEFAULT 0 NOT NULL
);

CREATE SEQUENCE public.tenant_invitations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.tenant_invitations_id_seq OWNED BY public.tenant_invitations.id;

CREATE TABLE public.tenant_members (
    id bigint NOT NULL,
    user_id character varying(36) NOT NULL,
    tenant_id integer NOT NULL,
    role character varying(20) DEFAULT 'contributor'::character varying NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    invited_by character varying(36),
    joined_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone
);

CREATE SEQUENCE public.tenant_members_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.tenant_members_id_seq OWNED BY public.tenant_members.id;

CREATE TABLE public.tenant_sandbox_configs (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    sandbox_type character varying(32) NOT NULL,
    config jsonb NOT NULL,
    cordoned_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE public.tenant_skill_catalog (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    version character varying(64),
    description text,
    instructions text,
    bundle_ref character varying(1024),
    bundle_sha256 character varying(64),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE public.tenant_skill_snapshots (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    sandbox_config_id character varying(36) NOT NULL,
    skill_id character varying(36),
    snapshot_id character varying(255),
    parent_snapshot_id character varying(255),
    generation integer DEFAULT 0 NOT NULL,
    trigger character varying(16) NOT NULL,
    state character varying(16) NOT NULL,
    superseded_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    planned_name character varying(255)
);

CREATE TABLE public.tenant_skills (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    sandbox_config_id character varying(36) NOT NULL,
    name character varying(255) NOT NULL,
    version character varying(64),
    description text,
    instructions text,
    bundle_ref character varying(1024),
    bundle_sha256 character varying(64),
    enabled boolean DEFAULT true NOT NULL,
    installed_snapshot_id character varying(255),
    status character varying(32) NOT NULL,
    error text,
    installing_since timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    install_session_id character varying(36),
    install_message_id character varying(36),
    envs jsonb,
    catalog_id character varying(36)
);

CREATE TABLE public.tenant_user_env_vars (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    principal_type character varying(32) NOT NULL,
    principal_id character varying(512) NOT NULL,
    sandbox_config_id character varying(36) NOT NULL,
    skill_id character varying(36) DEFAULT ''::character varying NOT NULL,
    name character varying(255) NOT NULL,
    value text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.tenants (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    retriever_engines jsonb DEFAULT '[]'::jsonb NOT NULL,
    status character varying(50) DEFAULT 'active'::character varying,
    business character varying(255) NOT NULL,
    storage_quota bigint DEFAULT '10737418240'::bigint NOT NULL,
    storage_used bigint DEFAULT 0 NOT NULL,
    agent_config jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    context_config jsonb,
    conversation_config jsonb,
    web_search_config jsonb,
    parser_engine_config jsonb,
    storage_engine_config jsonb,
    chat_history_config jsonb,
    retrieval_config jsonb,
    credentials jsonb,
    api_principal_config jsonb,
    default_storage_backend_id character varying(36),
    memory_config jsonb
);

CREATE SEQUENCE public.tenants_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.tenants_id_seq OWNED BY public.tenants.id;

CREATE TABLE public.user_kb_pins (
    tenant_id bigint NOT NULL,
    user_id character varying(36) NOT NULL,
    kb_id character varying(36) NOT NULL,
    pinned_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE public.user_resource_favorites (
    user_id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    resource_type character varying(16) NOT NULL,
    resource_id character varying(64) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE public.users (
    id character varying(36) DEFAULT uuid_generate_v4() NOT NULL,
    username character varying(100) NOT NULL,
    email character varying(255) NOT NULL,
    password_hash character varying(255) NOT NULL,
    avatar character varying(500),
    tenant_id integer,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    can_access_all_tenants boolean DEFAULT false NOT NULL,
    preferences jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_system_admin boolean DEFAULT false NOT NULL
);

CREATE TABLE public.utility_basic_accounts (
    id text NOT NULL,
    tenant_id bigint,
    category text,
    name text,
    account_no text,
    account_name text,
    usage_category text,
    voltage_level text,
    market_attr text,
    supply_unit text,
    address text,
    meter_no text,
    ratio numeric(18,4) DEFAULT 0,
    is_default boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

CREATE TABLE public.utility_basic_info (
    tenant_id bigint NOT NULL,
    category text NOT NULL,
    account_no text DEFAULT ''::text NOT NULL,
    account_name text DEFAULT ''::text NOT NULL,
    usage_category text DEFAULT ''::text NOT NULL,
    voltage_level text DEFAULT ''::text NOT NULL,
    market_attr text DEFAULT ''::text NOT NULL,
    supply_unit text DEFAULT ''::text NOT NULL,
    address text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.utility_field_configs (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    category character varying(20) NOT NULL,
    field_key character varying(64) NOT NULL,
    label character varying(128) NOT NULL,
    field_type character varying(20) DEFAULT 'text'::character varying NOT NULL,
    default_visible boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    is_custom boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    "group" text
);

CREATE TABLE public.utility_meter_items (
    id character varying(36) NOT NULL,
    record_id character varying(36) NOT NULL,
    meter_name character varying(128) NOT NULL,
    start_reading numeric(18,2) DEFAULT 0 NOT NULL,
    end_reading numeric(18,2) DEFAULT 0 NOT NULL,
    unit_price numeric(18,2) DEFAULT 0 NOT NULL,
    usage numeric(18,2) DEFAULT 0 NOT NULL,
    amount numeric(18,2) DEFAULT 0 NOT NULL,
    remark text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    meter_id character varying(36),
    reading_date character varying(20),
    reader character varying(100),
    rate numeric(12,4) DEFAULT 1,
    subsidy numeric(18,2) DEFAULT 0 NOT NULL,
    deep_prev numeric(18,2) DEFAULT 0 NOT NULL,
    deep_curr numeric(18,2) DEFAULT 0 NOT NULL,
    peak_prev numeric(18,2) DEFAULT 0 NOT NULL,
    peak_curr numeric(18,2) DEFAULT 0 NOT NULL,
    flat_prev numeric(18,2) DEFAULT 0 NOT NULL,
    flat_curr numeric(18,2) DEFAULT 0 NOT NULL,
    valley_prev numeric(18,2) DEFAULT 0 NOT NULL,
    valley_curr numeric(18,2) DEFAULT 0 NOT NULL
);

CREATE TABLE public.utility_meter_records (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    category character varying(20) NOT NULL,
    month character varying(7) NOT NULL,
    meter_count integer DEFAULT 0 NOT NULL,
    total_usage numeric(18,2) DEFAULT 0 NOT NULL,
    total_amount numeric(18,2) DEFAULT 0 NOT NULL,
    remark text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    record_date character varying(20)
);

CREATE TABLE public.utility_meters (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    category character varying(20) NOT NULL,
    alias character varying(128) NOT NULL,
    meter_no character varying(64),
    rate numeric(12,4) DEFAULT 1 NOT NULL,
    default_unit_price numeric(18,4) DEFAULT 0 NOT NULL,
    use_unit character varying(64),
    manager character varying(64),
    contact character varying(64),
    meter_mode character varying(20) DEFAULT 'manual'::character varying NOT NULL,
    install_date character varying(20),
    remark text,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    meter_kind character varying(20) DEFAULT 'dorm'::character varying NOT NULL,
    meter_type character varying(16) DEFAULT 'normal'::character varying NOT NULL,
    owner_unit character varying(200) DEFAULT ''::character varying NOT NULL
);

CREATE TABLE public.utility_tariff_rules (
    id character varying(36) NOT NULL,
    tenant_id bigint DEFAULT 10000 NOT NULL,
    category character varying(32) DEFAULT 'electricity'::character varying NOT NULL,
    name character varying(128) DEFAULT ''::character varying NOT NULL,
    months character varying(64) DEFAULT ''::character varying NOT NULL,
    deep_peak_rate numeric(18,4) DEFAULT 0 NOT NULL,
    peak_rate numeric(18,4) DEFAULT 0 NOT NULL,
    flat_rate numeric(18,4) DEFAULT 0 NOT NULL,
    valley_rate numeric(18,4) DEFAULT 0 NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE vector_stores (
    id character varying(36) NOT NULL,
    name character varying(255) NOT NULL,
    engine_type character varying(50) NOT NULL,
    connection_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    index_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    tenant_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);

CREATE TABLE public.web_search_providers (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    provider character varying(50) NOT NULL,
    description text,
    parameters jsonb,
    is_default boolean DEFAULT false,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);

CREATE TABLE public.wiki_folders (
    id character varying(36) NOT NULL,
    tenant_id bigint DEFAULT 0 NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    parent_id character varying(36) DEFAULT ''::character varying NOT NULL,
    name character varying(255) NOT NULL,
    path character varying(1024) DEFAULT ''::character varying NOT NULL,
    depth integer DEFAULT 0 NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE public.wiki_page_issues (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    slug character varying(255) NOT NULL,
    issue_type character varying(50) NOT NULL,
    description text NOT NULL,
    suspected_knowledge_ids jsonb,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    reported_by character varying(100) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone
);

CREATE TABLE public.wiki_page_revisions (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    page_id character varying(36) NOT NULL,
    slug character varying(255) NOT NULL,
    version integer NOT NULL,
    title character varying(512) DEFAULT ''::character varying NOT NULL,
    page_type character varying(32) DEFAULT 'summary'::character varying NOT NULL,
    status character varying(32) DEFAULT 'published'::character varying NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    summary text DEFAULT ''::text NOT NULL,
    aliases jsonb DEFAULT '[]'::jsonb,
    edit_source character varying(16) DEFAULT ''::character varying NOT NULL,
    editor_id character varying(64) DEFAULT ''::character varying NOT NULL,
    edited_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.wiki_pages (
    id character varying(36) NOT NULL,
    tenant_id bigint NOT NULL,
    knowledge_base_id character varying(36) NOT NULL,
    slug character varying(255) NOT NULL,
    title character varying(512) DEFAULT ''::character varying NOT NULL,
    page_type character varying(32) DEFAULT 'summary'::character varying NOT NULL,
    status character varying(32) DEFAULT 'published'::character varying NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    summary text DEFAULT ''::text NOT NULL,
    parent_slug character varying(255) DEFAULT ''::character varying NOT NULL,
    folder_id character varying(36) DEFAULT ''::character varying NOT NULL,
    category_path jsonb DEFAULT '[]'::jsonb,
    wiki_path character varying(1024) DEFAULT ''::character varying NOT NULL,
    depth integer DEFAULT 0 NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    source_refs jsonb DEFAULT '[]'::jsonb,
    chunk_refs jsonb DEFAULT '[]'::jsonb,
    in_links jsonb DEFAULT '[]'::jsonb,
    out_links jsonb DEFAULT '[]'::jsonb,
    page_metadata jsonb DEFAULT '{}'::jsonb,
    aliases jsonb DEFAULT '[]'::jsonb,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    last_edit_source character varying(16) DEFAULT ''::character varying NOT NULL,
    last_editor_id character varying(64) DEFAULT ''::character varying NOT NULL
);

ALTER TABLE ONLY public.audit_logs ALTER COLUMN id SET DEFAULT nextval('public.audit_logs_id_seq'::regclass);

ALTER TABLE ONLY public.embeddings ALTER COLUMN id SET DEFAULT nextval('public.embeddings_id_seq'::regclass);

ALTER TABLE ONLY public.knowledge_processing_spans ALTER COLUMN id SET DEFAULT nextval('public.knowledge_processing_spans_id_seq'::regclass);

ALTER TABLE ONLY public.message_suggestion_events ALTER COLUMN id SET DEFAULT nextval('public.message_suggestion_events_id_seq'::regclass);

ALTER TABLE ONLY public.system_settings ALTER COLUMN id SET DEFAULT nextval('public.system_settings_id_seq'::regclass);

ALTER TABLE ONLY public.task_dead_letters ALTER COLUMN id SET DEFAULT nextval('public.task_dead_letters_id_seq'::regclass);

ALTER TABLE ONLY public.task_pending_ops ALTER COLUMN id SET DEFAULT nextval('public.task_pending_ops_id_seq'::regclass);

ALTER TABLE ONLY public.tenant_api_keys ALTER COLUMN id SET DEFAULT nextval('public.tenant_api_keys_id_seq'::regclass);

ALTER TABLE ONLY public.tenant_invitations ALTER COLUMN id SET DEFAULT nextval('public.tenant_invitations_id_seq'::regclass);

ALTER TABLE ONLY public.tenant_members ALTER COLUMN id SET DEFAULT nextval('public.tenant_members_id_seq'::regclass);

ALTER TABLE ONLY public.tenants ALTER COLUMN id SET DEFAULT nextval('public.tenants_id_seq'::regclass);

SELECT pg_catalog.setval('public.audit_logs_id_seq', 337, true);

SELECT pg_catalog.setval('public.chunks_seq_id_seq', 100001401, true);

SELECT pg_catalog.setval('public.embeddings_id_seq', 1014, true);

SELECT pg_catalog.setval('public.knowledge_processing_spans_id_seq', 3143, true);

SELECT pg_catalog.setval('public.knowledge_tags_seq_id_seq', 10000168, true);

SELECT pg_catalog.setval('public.message_suggestion_events_id_seq', 1, false);

SELECT pg_catalog.setval('public.system_settings_id_seq', 1, false);

SELECT pg_catalog.setval('public.task_dead_letters_id_seq', 1, false);

SELECT pg_catalog.setval('public.task_pending_ops_id_seq', 1, false);

SELECT pg_catalog.setval('public.tenant_api_keys_id_seq', 1, false);

SELECT pg_catalog.setval('public.tenant_invitations_id_seq', 1, false);

SELECT pg_catalog.setval('public.tenant_members_id_seq', 2, true);

SELECT pg_catalog.setval('public.tenants_id_seq', 10001, true);

ALTER TABLE ONLY public.agent_shares
    ADD CONSTRAINT agent_shares_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.auth_tokens
    ADD CONSTRAINT auth_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_record_items
    ADD CONSTRAINT billing_record_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_records
    ADD CONSTRAINT billing_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_tenant_items
    ADD CONSTRAINT billing_tenant_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_tenant_meter_refs
    ADD CONSTRAINT billing_tenant_meter_refs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_tenant_settings
    ADD CONSTRAINT billing_tenant_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_tenants
    ADD CONSTRAINT billing_tenants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_time_meter_readings
    ADD CONSTRAINT billing_time_meter_readings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_time_meters
    ADD CONSTRAINT billing_time_meters_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_water_meter_readings
    ADD CONSTRAINT billing_water_meter_readings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_water_meters
    ADD CONSTRAINT billing_water_meters_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.chunk_revisions
    ADD CONSTRAINT chunk_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.chunks
    ADD CONSTRAINT chunks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.custom_agents
    ADD CONSTRAINT custom_agents_pkey PRIMARY KEY (id, tenant_id);

ALTER TABLE ONLY public.data_sources
    ADD CONSTRAINT data_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.embed_channels
    ADD CONSTRAINT embed_channels_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.embeddings
    ADD CONSTRAINT embeddings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.im_channel_sessions
    ADD CONSTRAINT im_channel_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.im_channels
    ADD CONSTRAINT im_channels_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.kb_shares
    ADD CONSTRAINT kb_shares_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.knowledge_bases
    ADD CONSTRAINT knowledge_bases_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.knowledge_processing_spans
    ADD CONSTRAINT knowledge_processing_spans_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.knowledge_tag_relations
    ADD CONSTRAINT knowledge_tag_relations_pkey PRIMARY KEY (knowledge_id, tag_id);

ALTER TABLE ONLY public.knowledge_tags
    ADD CONSTRAINT knowledge_tags_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.knowledges
    ADD CONSTRAINT knowledges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.mcp_oauth_clients
    ADD CONSTRAINT mcp_oauth_clients_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.mcp_oauth_tokens
    ADD CONSTRAINT mcp_oauth_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.mcp_services
    ADD CONSTRAINT mcp_services_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.mcp_tool_approvals
    ADD CONSTRAINT mcp_tool_approvals_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.memory_doc_affinity
    ADD CONSTRAINT memory_doc_affinity_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.memory_item_embeddings
    ADD CONSTRAINT memory_item_embeddings_pkey PRIMARY KEY (item_id);

ALTER TABLE ONLY public.memory_items
    ADD CONSTRAINT memory_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.memory_subjects
    ADD CONSTRAINT memory_subjects_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.memory_tombstones
    ADD CONSTRAINT memory_tombstones_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.memory_topic_stats
    ADD CONSTRAINT memory_topic_stats_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.message_suggestion_events
    ADD CONSTRAINT message_suggestion_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.message_suggestion_sets
    ADD CONSTRAINT message_suggestion_sets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.messages
    ADD CONSTRAINT messages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.models
    ADD CONSTRAINT models_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT organization_join_requests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.organization_members_pre_plan3
    ADD CONSTRAINT organization_members_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.organization_tenant_members
    ADD CONSTRAINT organization_tenant_members_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.resource_access_grants
    ADD CONSTRAINT resource_access_grants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.resource_access_grants
    ADD CONSTRAINT resource_access_grants_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY public.resource_bindings
    ADD CONSTRAINT resource_bindings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.resources
    ADD CONSTRAINT resources_handle_key UNIQUE (handle);

ALTER TABLE ONLY public.resources
    ADD CONSTRAINT resources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.storage_backends
    ADD CONSTRAINT storage_backends_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sync_logs
    ADD CONSTRAINT sync_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_key_key UNIQUE (key);

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.task_dead_letters
    ADD CONSTRAINT task_dead_letters_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.task_pending_ops
    ADD CONSTRAINT task_pending_ops_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.temporary_documents
    ADD CONSTRAINT temporary_documents_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tenant_api_keys
    ADD CONSTRAINT tenant_api_keys_key_hash_key UNIQUE (key_hash);

ALTER TABLE ONLY public.tenant_api_keys
    ADD CONSTRAINT tenant_api_keys_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tenant_disabled_shared_agents
    ADD CONSTRAINT tenant_disabled_shared_agents_pkey PRIMARY KEY (tenant_id, agent_id, source_tenant_id);

ALTER TABLE ONLY public.tenant_invitations
    ADD CONSTRAINT tenant_invitations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tenant_members
    ADD CONSTRAINT tenant_members_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tenant_sandbox_configs
    ADD CONSTRAINT tenant_sandbox_configs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tenant_skill_catalog
    ADD CONSTRAINT tenant_skill_catalog_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tenant_skill_snapshots
    ADD CONSTRAINT tenant_skill_snapshots_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tenant_skills
    ADD CONSTRAINT tenant_skills_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tenant_user_env_vars
    ADD CONSTRAINT tenant_user_env_vars_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tenants
    ADD CONSTRAINT tenants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.knowledge_processing_spans
    ADD CONSTRAINT uq_kpspan_attempt_span UNIQUE (knowledge_id, attempt, span_id);

ALTER TABLE ONLY public.billing_water_meter_readings
    ADD CONSTRAINT uq_water_meter_month UNIQUE (meter_id, month);

ALTER TABLE ONLY public.user_kb_pins
    ADD CONSTRAINT user_kb_pins_pkey PRIMARY KEY (tenant_id, user_id, kb_id);

ALTER TABLE ONLY public.user_resource_favorites
    ADD CONSTRAINT user_resource_favorites_pkey PRIMARY KEY (user_id, tenant_id, resource_type, resource_id);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);

ALTER TABLE ONLY public.utility_basic_accounts
    ADD CONSTRAINT utility_basic_accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.utility_basic_info
    ADD CONSTRAINT utility_basic_info_pkey PRIMARY KEY (tenant_id, category);

ALTER TABLE ONLY public.utility_field_configs
    ADD CONSTRAINT utility_field_configs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.utility_meter_items
    ADD CONSTRAINT utility_meter_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.utility_meter_records
    ADD CONSTRAINT utility_meter_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.utility_meters
    ADD CONSTRAINT utility_meters_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.utility_tariff_rules
    ADD CONSTRAINT utility_tariff_rules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY vector_stores
    ADD CONSTRAINT vector_stores_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.web_search_providers
    ADD CONSTRAINT web_search_providers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.wiki_folders
    ADD CONSTRAINT wiki_folders_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.wiki_page_issues
    ADD CONSTRAINT wiki_page_issues_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.wiki_page_revisions
    ADD CONSTRAINT wiki_page_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.wiki_pages
    ADD CONSTRAINT wiki_pages_pkey PRIMARY KEY (id);

CREATE INDEX embeddings_embedding_idx_1024 ON public.embeddings USING hnsw (((embedding)::halfvec(1024)) halfvec_cosine_ops) WITH (m='16', ef_construction='64') WHERE (dimension = 1024);

CREATE INDEX embeddings_embedding_idx_3584 ON public.embeddings USING hnsw (((embedding)::halfvec(3584)) halfvec_cosine_ops) WITH (m='16', ef_construction='64') WHERE (dimension = 3584);

CREATE INDEX embeddings_embedding_idx_798 ON public.embeddings USING hnsw (((embedding)::halfvec(798)) halfvec_cosine_ops) WITH (m='16', ef_construction='64') WHERE (dimension = 798);

CREATE UNIQUE INDEX embeddings_unique_source ON public.embeddings USING btree (source_id, source_type);

CREATE INDEX idx_agent_shares_agent_id ON public.agent_shares USING btree (agent_id);

CREATE UNIQUE INDEX idx_agent_shares_agent_org ON public.agent_shares USING btree (agent_id, source_tenant_id, organization_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_agent_shares_deleted_at ON public.agent_shares USING btree (deleted_at);

CREATE INDEX idx_agent_shares_org_id ON public.agent_shares USING btree (organization_id);

CREATE INDEX idx_agent_shares_source_tenant ON public.agent_shares USING btree (source_tenant_id);

CREATE INDEX idx_audit_logs_actor ON public.audit_logs USING btree (actor_user_id);

CREATE INDEX idx_audit_logs_created_at ON public.audit_logs USING btree (created_at);

CREATE INDEX idx_audit_logs_tenant_action ON public.audit_logs USING btree (tenant_id, action);

CREATE INDEX idx_audit_logs_tenant_id_desc ON public.audit_logs USING btree (tenant_id, id DESC);

CREATE INDEX idx_audit_logs_tenant_scope_desc ON public.audit_logs USING btree (tenant_id, scope_type, scope_id, id DESC);

CREATE INDEX idx_auth_tokens_expires_at ON public.auth_tokens USING btree (expires_at);

CREATE INDEX idx_auth_tokens_token ON public.auth_tokens USING btree (token);

CREATE INDEX idx_auth_tokens_token_type ON public.auth_tokens USING btree (token_type);

CREATE INDEX idx_auth_tokens_user_id ON public.auth_tokens USING btree (user_id);

CREATE UNIQUE INDEX idx_channel_lookup ON public.im_channel_sessions USING btree (platform, user_id, chat_id, tenant_id, agent_id) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX idx_channel_thread_lookup ON public.im_channel_sessions USING btree (platform, chat_id, thread_id, tenant_id, agent_id) WHERE ((deleted_at IS NULL) AND ((thread_id)::text <> ''::text));

CREATE UNIQUE INDEX idx_chunk_revisions_chunk_revision ON public.chunk_revisions USING btree (chunk_id, revision);

CREATE INDEX idx_chunk_revisions_tenant_chunk ON public.chunk_revisions USING btree (tenant_id, chunk_id);

CREATE INDEX idx_chunks_chunk_type ON public.chunks USING btree (chunk_type);

CREATE INDEX idx_chunks_content_hash ON public.chunks USING btree (content_hash);

CREATE INDEX idx_chunks_kb_tenant ON public.chunks USING btree (knowledge_base_id, tenant_id);

CREATE INDEX idx_chunks_knowledge_enabled ON public.chunks USING btree (knowledge_id, is_enabled, deleted_at);

CREATE INDEX idx_chunks_parent_id ON public.chunks USING btree (parent_chunk_id);

CREATE UNIQUE INDEX idx_chunks_seq_id ON public.chunks USING btree (seq_id);

CREATE INDEX idx_chunks_tag ON public.chunks USING btree (tag_id);

CREATE INDEX idx_chunks_tenant_kg ON public.chunks USING btree (tenant_id, knowledge_id);

CREATE INDEX idx_custom_agents_deleted_at ON public.custom_agents USING btree (deleted_at);

CREATE INDEX idx_custom_agents_is_builtin ON public.custom_agents USING btree (is_builtin);

CREATE INDEX idx_custom_agents_tenant_id ON public.custom_agents USING btree (tenant_id);

CREATE INDEX idx_data_sources_deleted_at ON public.data_sources USING btree (deleted_at);

CREATE INDEX idx_data_sources_knowledge_base_id ON public.data_sources USING btree (knowledge_base_id);

CREATE INDEX idx_data_sources_status ON public.data_sources USING btree (status);

CREATE INDEX idx_data_sources_tenant_id ON public.data_sources USING btree (tenant_id);

CREATE INDEX idx_data_sources_type ON public.data_sources USING btree (type);

CREATE INDEX idx_embed_channels_agent ON public.embed_channels USING btree (agent_id);

CREATE INDEX idx_embed_channels_deleted ON public.embed_channels USING btree (deleted_at) WHERE (deleted_at IS NOT NULL);

CREATE UNIQUE INDEX idx_embed_channels_publish_token ON public.embed_channels USING btree (publish_token) WHERE (((publish_token)::text <> ''::text) AND (deleted_at IS NULL));

CREATE INDEX idx_embed_channels_tenant ON public.embed_channels USING btree (tenant_id);

CREATE INDEX idx_embeddings_is_enabled ON public.embeddings USING btree (is_enabled);

CREATE INDEX idx_embeddings_knowledge_base_id ON public.embeddings USING btree (knowledge_base_id);

CREATE INDEX idx_embeddings_tag_id ON public.embeddings USING btree (tag_id);

CREATE INDEX idx_im_channel_deleted ON public.im_channel_sessions USING btree (deleted_at) WHERE (deleted_at IS NOT NULL);

CREATE INDEX idx_im_channel_session ON public.im_channel_sessions USING btree (session_id);

CREATE INDEX idx_im_channel_sessions_channel ON public.im_channel_sessions USING btree (im_channel_id) WHERE ((im_channel_id)::text <> ''::text);

CREATE INDEX idx_im_channel_tenant ON public.im_channel_sessions USING btree (tenant_id);

CREATE INDEX idx_im_channels_agent ON public.im_channels USING btree (agent_id);

CREATE UNIQUE INDEX idx_im_channels_bot_identity ON public.im_channels USING btree (bot_identity) WHERE ((deleted_at IS NULL) AND ((bot_identity)::text <> ''::text));

CREATE INDEX idx_im_channels_deleted ON public.im_channels USING btree (deleted_at) WHERE (deleted_at IS NOT NULL);

CREATE INDEX idx_im_channels_tenant ON public.im_channels USING btree (tenant_id);

CREATE INDEX idx_kb_shares_deleted_at ON public.kb_shares USING btree (deleted_at);

CREATE INDEX idx_kb_shares_kb_id ON public.kb_shares USING btree (knowledge_base_id);

CREATE UNIQUE INDEX idx_kb_shares_kb_org ON public.kb_shares USING btree (knowledge_base_id, organization_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_kb_shares_org_id ON public.kb_shares USING btree (organization_id);

CREATE INDEX idx_kb_shares_source_tenant ON public.kb_shares USING btree (source_tenant_id);

CREATE INDEX idx_knowledge_bases_storage_backend ON public.knowledge_bases USING btree (tenant_id, storage_backend_id);

CREATE INDEX idx_knowledge_bases_tenant_creator ON public.knowledge_bases USING btree (tenant_id, creator_id);

CREATE INDEX idx_knowledge_bases_tenant_id ON public.knowledge_bases USING btree (tenant_id);

CREATE INDEX idx_knowledge_bases_tenant_vector_store ON public.knowledge_bases USING btree (tenant_id, vector_store_id);

CREATE INDEX idx_knowledge_tags_kb ON public.knowledge_tags USING btree (tenant_id, knowledge_base_id);

CREATE UNIQUE INDEX idx_knowledge_tags_kb_name ON public.knowledge_tags USING btree (tenant_id, knowledge_base_id, name);

CREATE UNIQUE INDEX idx_knowledge_tags_seq_id ON public.knowledge_tags USING btree (seq_id);

CREATE INDEX idx_knowledges_base_id ON public.knowledges USING btree (knowledge_base_id);

CREATE INDEX idx_knowledges_enable_status ON public.knowledges USING btree (enable_status);

CREATE INDEX idx_knowledges_folder_path ON public.knowledges USING btree (tenant_id, knowledge_base_id, folder_path);

CREATE INDEX idx_knowledges_kb_metadata_external_id ON public.knowledges USING btree (knowledge_base_id, ((metadata ->> 'external_id'::text)) text_pattern_ops) WHERE (deleted_at IS NULL);

CREATE INDEX idx_knowledges_parse_status ON public.knowledges USING btree (parse_status);

CREATE INDEX idx_knowledges_summary_status ON public.knowledges USING btree (summary_status);

CREATE INDEX idx_knowledges_tenant_id ON public.knowledges USING btree (tenant_id);

CREATE INDEX idx_kpspan_knowledge_attempt ON public.knowledge_processing_spans USING btree (knowledge_id, attempt);

CREATE INDEX idx_kpspan_parent ON public.knowledge_processing_spans USING btree (parent_span_id) WHERE (parent_span_id IS NOT NULL);

CREATE INDEX idx_kpspan_status_started ON public.knowledge_processing_spans USING btree (status, started_at);

CREATE INDEX idx_ktr_knowledge ON public.knowledge_tag_relations USING btree (knowledge_id);

CREATE INDEX idx_ktr_tag ON public.knowledge_tag_relations USING btree (tag_id);

CREATE INDEX idx_mcp_oauth_clients_service_id ON public.mcp_oauth_clients USING btree (service_id);

CREATE UNIQUE INDEX idx_mcp_oauth_clients_tenant_svc ON public.mcp_oauth_clients USING btree (tenant_id, service_id);

CREATE INDEX idx_mcp_oauth_tokens_principal ON public.mcp_oauth_tokens USING btree (principal_type, principal_id);

CREATE INDEX idx_mcp_oauth_tokens_service_id ON public.mcp_oauth_tokens USING btree (service_id);

CREATE UNIQUE INDEX idx_mcp_oauth_tokens_tenant_principal_svc ON public.mcp_oauth_tokens USING btree (tenant_id, principal_type, principal_id, service_id);

CREATE INDEX idx_mcp_oauth_tokens_user_id ON public.mcp_oauth_tokens USING btree (user_id);

CREATE INDEX idx_mcp_services_deleted_at ON public.mcp_services USING btree (deleted_at);

CREATE INDEX idx_mcp_services_enabled ON public.mcp_services USING btree (enabled);

CREATE INDEX idx_mcp_services_is_builtin ON public.mcp_services USING btree (is_builtin);

CREATE INDEX idx_mcp_services_tenant_id ON public.mcp_services USING btree (tenant_id);

CREATE INDEX idx_mcp_tool_approvals_service_id ON public.mcp_tool_approvals USING btree (service_id);

CREATE UNIQUE INDEX idx_mcp_tool_approvals_tenant_svc_tool ON public.mcp_tool_approvals USING btree (tenant_id, service_id, tool_name);

CREATE UNIQUE INDEX idx_mem_affinity_scope ON public.memory_doc_affinity USING btree (tenant_id, subject_id, knowledge_id);

CREATE INDEX idx_mem_emb_scope ON public.memory_item_embeddings USING btree (tenant_id, subject_id);

CREATE UNIQUE INDEX idx_mem_tomb_fp ON public.memory_tombstones USING btree (tenant_id, subject_id, fingerprint);

CREATE UNIQUE INDEX idx_mem_topic_scope ON public.memory_topic_stats USING btree (tenant_id, subject_id, normalized_key);

CREATE INDEX idx_memory_items_key ON public.memory_items USING btree (tenant_id, subject_id, normalized_key);

CREATE INDEX idx_memory_items_scope ON public.memory_items USING btree (tenant_id, subject_id, status);

CREATE UNIQUE INDEX idx_memory_subjects_scope ON public.memory_subjects USING btree (tenant_id, subject_id);

CREATE INDEX idx_memory_tombstones_scope ON public.memory_tombstones USING btree (tenant_id, subject_id);

CREATE INDEX idx_message_suggestion_events_session ON public.message_suggestion_events USING btree (tenant_id, session_id, created_at);

CREATE INDEX idx_message_suggestion_events_set ON public.message_suggestion_events USING btree (suggestion_set_id, created_at);

CREATE INDEX idx_message_suggestion_events_type ON public.message_suggestion_events USING btree (event_type, created_at);

CREATE UNIQUE INDEX idx_message_suggestion_sets_cache_key ON public.message_suggestion_sets USING btree (tenant_id, assistant_message_id, placement, config_hash, locale);

CREATE INDEX idx_message_suggestion_sets_session ON public.message_suggestion_sets USING btree (tenant_id, session_id, created_at DESC);

CREATE INDEX idx_message_suggestion_sets_status ON public.message_suggestion_sets USING btree (status, lease_until);

CREATE INDEX idx_messages_agent_id ON public.messages USING btree (agent_id);

CREATE INDEX idx_messages_agent_steps ON public.messages USING gin (agent_steps);

CREATE INDEX idx_messages_knowledge_id ON public.messages USING btree (knowledge_id);

CREATE INDEX idx_messages_session_id ON public.messages USING btree (session_id);

CREATE INDEX idx_models_is_builtin ON public.models USING btree (is_builtin);

CREATE INDEX idx_models_managed_by_yaml ON public.models USING btree (managed_by) WHERE ((managed_by)::text <> ''::text);

CREATE INDEX idx_models_source ON public.models USING btree (source);

CREATE INDEX idx_models_type ON public.models USING btree (type);

CREATE INDEX idx_org_join_requests_org_id ON public.organization_join_requests USING btree (organization_id);

CREATE UNIQUE INDEX idx_org_join_requests_org_user_pending ON public.organization_join_requests USING btree (organization_id, user_id) WHERE ((status)::text = 'pending'::text);

CREATE INDEX idx_org_join_requests_status ON public.organization_join_requests USING btree (status);

CREATE INDEX idx_org_join_requests_type ON public.organization_join_requests USING btree (request_type);

CREATE INDEX idx_org_join_requests_user_id ON public.organization_join_requests USING btree (user_id);

CREATE UNIQUE INDEX idx_org_members_org_user_pre_plan3 ON public.organization_members_pre_plan3 USING btree (organization_id, user_id);

CREATE INDEX idx_org_members_role_pre_plan3 ON public.organization_members_pre_plan3 USING btree (role);

CREATE INDEX idx_org_members_tenant_id_pre_plan3 ON public.organization_members_pre_plan3 USING btree (tenant_id);

CREATE INDEX idx_org_members_user_id_pre_plan3 ON public.organization_members_pre_plan3 USING btree (user_id);

CREATE INDEX idx_org_tenant_members_by_tenant ON public.organization_tenant_members USING btree (tenant_id);

CREATE INDEX idx_org_tenant_members_role ON public.organization_tenant_members USING btree (organization_id, role);

CREATE UNIQUE INDEX idx_org_tenant_members_unique ON public.organization_tenant_members USING btree (organization_id, tenant_id);

CREATE INDEX idx_organizations_deleted_at ON public.organizations USING btree (deleted_at);

CREATE UNIQUE INDEX idx_organizations_invite_code ON public.organizations USING btree (invite_code) WHERE ((invite_code IS NOT NULL) AND (deleted_at IS NULL));

CREATE INDEX idx_organizations_owner_id ON public.organizations USING btree (owner_id);

CREATE INDEX idx_organizations_owner_tenant ON public.organizations USING btree (owner_tenant_id);

CREATE INDEX idx_resource_access_grants_expires ON public.resource_access_grants USING btree (expires_at);

CREATE INDEX idx_resource_access_grants_resource ON public.resource_access_grants USING btree (resource_id);

CREATE INDEX idx_resource_bindings_owner ON public.resource_bindings USING btree (tenant_id, owner_type, owner_id);

CREATE UNIQUE INDEX idx_resource_bindings_unique ON public.resource_bindings USING btree (resource_id, owner_type, owner_id, relation);

CREATE INDEX idx_resources_backend ON public.resources USING btree (storage_backend_id);

CREATE INDEX idx_resources_tenant ON public.resources USING btree (tenant_id);

CREATE UNIQUE INDEX idx_resources_tenant_location ON public.resources USING btree (tenant_id, location_hash) WHERE (deleted_at IS NULL);

CREATE INDEX idx_sessions_agent_config ON public.sessions USING gin (agent_config);

CREATE INDEX idx_sessions_agent_id ON public.sessions USING btree (agent_id);

CREATE INDEX idx_sessions_context_config ON public.sessions USING gin (context_config);

CREATE INDEX idx_sessions_tenant_id ON public.sessions USING btree (tenant_id);

CREATE INDEX idx_sessions_tenant_user_pin ON public.sessions USING btree (tenant_id, user_id, is_pinned DESC, pinned_at DESC, updated_at DESC) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX idx_storage_backends_legacy_alias ON public.storage_backends USING btree (tenant_id, provider) WHERE ((deleted_at IS NULL) AND (legacy_alias = true));

CREATE UNIQUE INDEX idx_storage_backends_name_tenant ON public.storage_backends USING btree (tenant_id, name) WHERE (deleted_at IS NULL);

CREATE INDEX idx_storage_backends_tenant ON public.storage_backends USING btree (tenant_id);

CREATE INDEX idx_sync_logs_data_source_id ON public.sync_logs USING btree (data_source_id);

CREATE INDEX idx_sync_logs_started_at ON public.sync_logs USING btree (started_at);

CREATE INDEX idx_sync_logs_status ON public.sync_logs USING btree (status);

CREATE INDEX idx_sync_logs_tenant_id ON public.sync_logs USING btree (tenant_id);

CREATE INDEX idx_system_settings_category ON public.system_settings USING btree (category);

CREATE INDEX idx_task_dead_letters_scope ON public.task_dead_letters USING btree (scope, scope_id, failed_at DESC);

CREATE INDEX idx_task_dead_letters_task_type ON public.task_dead_letters USING btree (task_type, failed_at DESC);

CREATE INDEX idx_task_dead_letters_tenant ON public.task_dead_letters USING btree (tenant_id, failed_at DESC);

CREATE INDEX idx_task_pending_ops_scope ON public.task_pending_ops USING btree (task_type, scope, scope_id, id);

CREATE INDEX idx_task_pending_ops_tenant ON public.task_pending_ops USING btree (tenant_id);

CREATE INDEX idx_temporary_documents_expires ON public.temporary_documents USING btree (expires_at);

CREATE INDEX idx_temporary_documents_scope ON public.temporary_documents USING btree (tenant_id, session_id);

CREATE INDEX idx_temporary_documents_status ON public.temporary_documents USING btree (status);

CREATE INDEX idx_tenant_api_keys_revoked_at ON public.tenant_api_keys USING btree (revoked_at);

CREATE INDEX idx_tenant_api_keys_scope_type ON public.tenant_api_keys USING btree (scope_type);

CREATE INDEX idx_tenant_api_keys_tenant ON public.tenant_api_keys USING btree (tenant_id);

CREATE INDEX idx_tenant_disabled_shared_agents_tenant_id ON public.tenant_disabled_shared_agents USING btree (tenant_id);

CREATE INDEX idx_tenant_invitations_invitee ON public.tenant_invitations USING btree (invitee_user_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_tenant_invitations_tenant ON public.tenant_invitations USING btree (tenant_id) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX idx_tenant_invitations_token ON public.tenant_invitations USING btree (token) WHERE (((token)::text <> ''::text) AND (deleted_at IS NULL));

CREATE UNIQUE INDEX idx_tenant_invitations_unique_pending ON public.tenant_invitations USING btree (tenant_id, invitee_user_id) WHERE (((status)::text = 'pending'::text) AND (deleted_at IS NULL) AND ((invitee_user_id)::text <> ''::text));

CREATE INDEX idx_tenant_members_tenant_role ON public.tenant_members USING btree (tenant_id, role) WHERE (deleted_at IS NULL);

CREATE INDEX idx_tenant_members_user ON public.tenant_members USING btree (user_id) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX idx_tenant_members_user_tenant_unique ON public.tenant_members USING btree (user_id, tenant_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_tenant_sandbox_configs_tenant ON public.tenant_sandbox_configs USING btree (tenant_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_tenant_skill_snapshots_config ON public.tenant_skill_snapshots USING btree (sandbox_config_id);

CREATE INDEX idx_tenant_skill_snapshots_state ON public.tenant_skill_snapshots USING btree (state);

CREATE INDEX idx_tenant_skills_catalog ON public.tenant_skills USING btree (catalog_id);

CREATE INDEX idx_tenants_agent_config ON public.tenants USING gin (agent_config);

CREATE INDEX idx_tenants_status ON public.tenants USING btree (status);

CREATE INDEX idx_uba_deleted ON public.utility_basic_accounts USING btree (deleted_at);

CREATE INDEX idx_uba_tenant_cat ON public.utility_basic_accounts USING btree (tenant_id, category);

CREATE INDEX idx_user_env_var_config ON public.tenant_user_env_vars USING btree (tenant_id, sandbox_config_id);

CREATE INDEX idx_user_env_var_skill ON public.tenant_user_env_vars USING btree (tenant_id, skill_id);

CREATE INDEX idx_user_kb_pins_user_tenant_pinned_at ON public.user_kb_pins USING btree (tenant_id, user_id, pinned_at DESC);

CREATE INDEX idx_user_resource_favorites_tenant_id ON public.user_resource_favorites USING btree (tenant_id);

CREATE INDEX idx_user_resource_favorites_user_tenant_type_created_at ON public.user_resource_favorites USING btree (user_id, tenant_id, resource_type, created_at DESC);

CREATE INDEX idx_users_deleted_at ON public.users USING btree (deleted_at);

CREATE INDEX idx_users_email ON public.users USING btree (email);

CREATE INDEX idx_users_is_system_admin ON public.users USING btree (is_system_admin);

CREATE INDEX idx_users_tenant_id ON public.users USING btree (tenant_id);

CREATE INDEX idx_users_username ON public.users USING btree (username);

CREATE INDEX idx_utility_meter_items_meter ON public.utility_meter_items USING btree (meter_id);

CREATE INDEX idx_utility_meter_items_record ON public.utility_meter_items USING btree (record_id);

CREATE INDEX idx_utility_meter_records_tenant_cat ON public.utility_meter_records USING btree (tenant_id, category);

CREATE INDEX idx_utility_meters_tenant_cat ON public.utility_meters USING btree (tenant_id, category);

CREATE INDEX idx_utility_tariff_rules_tenant ON public.utility_tariff_rules USING btree (tenant_id, category);

CREATE INDEX idx_vector_stores_deleted_at ON vector_stores USING btree (deleted_at);

CREATE INDEX idx_vector_stores_engine_type ON vector_stores USING btree (engine_type);

CREATE UNIQUE INDEX idx_vector_stores_name_tenant ON vector_stores USING btree (name, tenant_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_vector_stores_tenant_id ON vector_stores USING btree (tenant_id);

CREATE INDEX idx_water_meters_tenant ON public.billing_water_meters USING btree (billing_tenant_id, deleted_at);

CREATE INDEX idx_web_search_providers_deleted_at ON public.web_search_providers USING btree (deleted_at);

CREATE INDEX idx_web_search_providers_provider ON public.web_search_providers USING btree (provider);

CREATE INDEX idx_web_search_providers_tenant_id ON public.web_search_providers USING btree (tenant_id);

CREATE INDEX idx_wiki_folders_deleted_at ON public.wiki_folders USING btree (deleted_at);

CREATE INDEX idx_wiki_folders_parent ON public.wiki_folders USING btree (knowledge_base_id, parent_id);

CREATE UNIQUE INDEX idx_wiki_folders_parent_name ON public.wiki_folders USING btree (knowledge_base_id, parent_id, name) WHERE (deleted_at IS NULL);

CREATE INDEX idx_wiki_page_issues_knowledge_base_id ON public.wiki_page_issues USING btree (knowledge_base_id);

CREATE INDEX idx_wiki_page_issues_slug ON public.wiki_page_issues USING btree (slug);

CREATE INDEX idx_wiki_page_issues_status ON public.wiki_page_issues USING btree (status);

CREATE INDEX idx_wiki_page_issues_tenant_id ON public.wiki_page_issues USING btree (tenant_id);

CREATE INDEX idx_wiki_page_revisions_kb_slug ON public.wiki_page_revisions USING btree (knowledge_base_id, slug);

CREATE UNIQUE INDEX idx_wiki_page_revisions_page_version ON public.wiki_page_revisions USING btree (page_id, version);

CREATE INDEX idx_wiki_pages_deleted_at ON public.wiki_pages USING btree (deleted_at);

CREATE INDEX idx_wiki_pages_folder ON public.wiki_pages USING btree (knowledge_base_id, folder_id);

CREATE INDEX idx_wiki_pages_folder_id ON public.wiki_pages USING btree (folder_id);

CREATE INDEX idx_wiki_pages_fulltext ON public.wiki_pages USING gin (to_tsvector('simple'::regconfig, (((COALESCE(title, ''::character varying))::text || ' '::text) || COALESCE(content, ''::text))));

CREATE INDEX idx_wiki_pages_kb_id ON public.wiki_pages USING btree (knowledge_base_id);

CREATE UNIQUE INDEX idx_wiki_pages_kb_slug ON public.wiki_pages USING btree (knowledge_base_id, slug) WHERE (deleted_at IS NULL);

CREATE INDEX idx_wiki_pages_page_type ON public.wiki_pages USING btree (knowledge_base_id, page_type);

CREATE INDEX idx_wiki_pages_parent_slug ON public.wiki_pages USING btree (knowledge_base_id, parent_slug);

CREATE INDEX idx_wiki_pages_source_refs ON public.wiki_pages USING gin (source_refs jsonb_path_ops);

CREATE INDEX idx_wiki_pages_source_refs_text ON public.wiki_pages USING gin (to_tsvector('simple'::regconfig, (source_refs)::text));

CREATE INDEX idx_wiki_pages_tenant_id ON public.wiki_pages USING btree (tenant_id);

CREATE INDEX idx_wiki_pages_title_trgm ON public.wiki_pages USING gin (lower((title)::text) public.gin_trgm_ops);

CREATE INDEX idx_wiki_pages_tree ON public.wiki_pages USING btree (knowledge_base_id, page_type, wiki_path, sort_order, title);

CREATE UNIQUE INDEX uq_billing_records ON public.billing_records USING btree (billing_tenant_id, month) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX uq_billing_tenant_items ON public.billing_tenant_items USING btree (billing_tenant_id, item_key);

CREATE UNIQUE INDEX uq_billing_tenant_meter_refs ON public.billing_tenant_meter_refs USING btree (billing_tenant_id, category, meter_id);

CREATE UNIQUE INDEX uq_billing_time_reading ON public.billing_time_meter_readings USING btree (meter_id, month);

CREATE UNIQUE INDEX uq_org_join_requests_pending_per_tenant ON public.organization_join_requests USING btree (organization_id, tenant_id, request_type) WHERE ((status)::text = 'pending'::text);

CREATE UNIQUE INDEX uq_tenant_sandbox_configs_tenant_name ON public.tenant_sandbox_configs USING btree (tenant_id, name) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX uq_tenant_skill_catalog_name ON public.tenant_skill_catalog USING btree (tenant_id, name) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX uq_tenant_skills_config_name ON public.tenant_skills USING btree (sandbox_config_id, name) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX uq_user_env_var ON public.tenant_user_env_vars USING btree (tenant_id, principal_type, principal_id, sandbox_config_id, skill_id, name);

CREATE UNIQUE INDEX uq_utility_field_configs ON public.utility_field_configs USING btree (tenant_id, category, field_key) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX uq_utility_meter_records ON public.utility_meter_records USING btree (tenant_id, category, month) WHERE (deleted_at IS NULL);

CREATE TRIGGER trigger_mcp_services_updated_at BEFORE UPDATE ON public.mcp_services FOR EACH ROW EXECUTE FUNCTION public.update_mcp_services_updated_at();

ALTER TABLE ONLY public.agent_shares
    ADD CONSTRAINT agent_shares_agent_id_source_tenant_id_fkey FOREIGN KEY (agent_id, source_tenant_id) REFERENCES public.custom_agents(id, tenant_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_shares
    ADD CONSTRAINT agent_shares_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.auth_tokens
    ADD CONSTRAINT fk_auth_tokens_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.users
    ADD CONSTRAINT fk_users_tenant FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.im_channel_sessions
    ADD CONSTRAINT im_channel_sessions_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.kb_shares
    ADD CONSTRAINT kb_shares_knowledge_base_id_fkey FOREIGN KEY (knowledge_base_id) REFERENCES public.knowledge_bases(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.kb_shares
    ADD CONSTRAINT kb_shares_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_oauth_clients
    ADD CONSTRAINT mcp_oauth_clients_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.mcp_services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_oauth_tokens
    ADD CONSTRAINT mcp_oauth_tokens_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.mcp_services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_tool_approvals
    ADD CONSTRAINT mcp_tool_approvals_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.mcp_services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.message_suggestion_events
    ADD CONSTRAINT message_suggestion_events_suggestion_set_id_fkey FOREIGN KEY (suggestion_set_id) REFERENCES public.message_suggestion_sets(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.message_suggestion_events
    ADD CONSTRAINT message_suggestion_events_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.message_suggestion_sets
    ADD CONSTRAINT message_suggestion_sets_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT organization_join_requests_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.organization_members_pre_plan3
    ADD CONSTRAINT organization_members_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.organization_tenant_members
    ADD CONSTRAINT organization_tenant_members_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.resource_access_grants
    ADD CONSTRAINT resource_access_grants_resource_id_fkey FOREIGN KEY (resource_id) REFERENCES public.resources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.resource_bindings
    ADD CONSTRAINT resource_bindings_resource_id_fkey FOREIGN KEY (resource_id) REFERENCES public.resources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sync_logs
    ADD CONSTRAINT sync_logs_data_source_id_fkey FOREIGN KEY (data_source_id) REFERENCES public.data_sources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.tenant_api_keys
    ADD CONSTRAINT tenant_api_keys_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;
