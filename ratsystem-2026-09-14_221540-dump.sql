--
-- PostgreSQL database dump
--

\restrict uJ2GW4CG8nNogm38dcFhLKatnF01weITrJ2GH0VNNu4aLAmzXist1iX0LcokZ1c

-- Dumped from database version 18.4
-- Dumped by pg_dump version 18.4

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: pg_database_owner
--

CREATE SCHEMA public;


ALTER SCHEMA public OWNER TO pg_database_owner;

--
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: pg_database_owner
--

COMMENT ON SCHEMA public IS 'standard public schema';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: agents; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.agents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    room_id uuid,
    hostname character varying(255) NOT NULL,
    os_info jsonb,
    mac_address character varying(17),
    ip_address inet,
    status character varying(20) DEFAULT 'OFFLINE'::character varying NOT NULL,
    last_seen timestamp with time zone,
    enrolled_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agents_status_check CHECK (((status)::text = ANY ((ARRAY['ONLINE'::character varying, 'OFFLINE'::character varying, 'WARNING'::character varying, 'DISABLED'::character varying])::text[])))
);


ALTER TABLE public.agents OWNER TO postgres;

--
-- Name: av_jobs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.av_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    requested_by uuid NOT NULL,
    scan_type character varying(30) NOT NULL,
    path text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.av_jobs OWNER TO postgres;

--
-- Name: av_scan_results; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.av_scan_results (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    command_id uuid NOT NULL,
    scan_type character varying(30) NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    total_files_scanned bigint DEFAULT 0 NOT NULL,
    threats_found integer DEFAULT 0 NOT NULL,
    threat_details jsonb,
    status character varying(30) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    job_id uuid NOT NULL,
    CONSTRAINT av_scan_results_status_check CHECK (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'RUNNING'::character varying, 'COMPLETED'::character varying, 'FAILED'::character varying, 'CANCELLED'::character varying])::text[]))),
    CONSTRAINT av_scan_results_threats_found_check CHECK ((threats_found >= 0)),
    CONSTRAINT av_scan_results_total_files_scanned_check CHECK ((total_files_scanned >= 0))
);


ALTER TABLE public.av_scan_results OWNER TO postgres;

--
-- Name: commands; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.commands (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    issued_by uuid NOT NULL,
    command_type character varying(100) NOT NULL,
    payload jsonb,
    status character varying(30) DEFAULT 'QUEUED'::character varying NOT NULL,
    result_message text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    CONSTRAINT commands_status_check CHECK (((status)::text = ANY ((ARRAY['QUEUED'::character varying, 'DELIVERED'::character varying, 'ACKNOWLEDGED'::character varying, 'RUNNING'::character varying, 'SUCCEEDED'::character varying, 'FAILED'::character varying, 'CANCELLED'::character varying, 'EXPIRED'::character varying])::text[])))
);


ALTER TABLE public.commands OWNER TO postgres;

--
-- Name: file_distribution_jobs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.file_distribution_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    file_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    request_id uuid,
    target_type character varying(10) NOT NULL,
    room_id uuid,
    status character varying(30) DEFAULT 'PENDING'::character varying NOT NULL,
    total_targets integer DEFAULT 0 NOT NULL,
    completed_targets integer DEFAULT 0 NOT NULL,
    failed_targets integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT file_distribution_jobs_completed_targets_check CHECK ((completed_targets >= 0)),
    CONSTRAINT file_distribution_jobs_failed_targets_check CHECK ((failed_targets >= 0)),
    CONSTRAINT file_distribution_jobs_status_check CHECK (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'DISPATCHING'::character varying, 'IN_PROGRESS'::character varying, 'COMPLETED'::character varying, 'PARTIAL_FAILED'::character varying, 'FAILED'::character varying, 'CANCELLED'::character varying])::text[]))),
    CONSTRAINT file_distribution_jobs_target CHECK (((((target_type)::text = 'ROOM'::text) AND (room_id IS NOT NULL)) OR (((target_type)::text = 'AGENTS'::text) AND (room_id IS NULL)))),
    CONSTRAINT file_distribution_jobs_target_type_check CHECK (((target_type)::text = ANY ((ARRAY['ROOM'::character varying, 'AGENTS'::character varying])::text[]))),
    CONSTRAINT file_distribution_jobs_total_targets_check CHECK ((total_targets >= 0))
);


ALTER TABLE public.file_distribution_jobs OWNER TO postgres;

--
-- Name: file_distribution_targets; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.file_distribution_targets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    job_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    status character varying(30) DEFAULT 'PENDING'::character varying NOT NULL,
    progress smallint DEFAULT 0 NOT NULL,
    downloaded_bytes bigint DEFAULT 0 NOT NULL,
    error_code character varying(100),
    error_message text,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT file_distribution_targets_downloaded_bytes_check CHECK ((downloaded_bytes >= 0)),
    CONSTRAINT file_distribution_targets_progress_check CHECK (((progress >= 0) AND (progress <= 100))),
    CONSTRAINT file_distribution_targets_status_check CHECK (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'SENT'::character varying, 'DOWNLOADING'::character varying, 'VERIFYING'::character varying, 'COMPLETED'::character varying, 'FAILED'::character varying, 'OFFLINE'::character varying, 'CANCELLED'::character varying])::text[])))
);


ALTER TABLE public.file_distribution_targets OWNER TO postgres;

--
-- Name: file_download_grants; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.file_download_grants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    token_hash character(64) NOT NULL,
    job_id uuid NOT NULL,
    file_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.file_download_grants OWNER TO postgres;

--
-- Name: files; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.files (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    filename character varying(255) NOT NULL,
    original_name character varying(255) NOT NULL,
    file_size bigint NOT NULL,
    storage_path text NOT NULL,
    hash_sha256 character(64) NOT NULL,
    uploaded_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT files_file_size_check CHECK ((file_size >= 0))
);


ALTER TABLE public.files OWNER TO postgres;

--
-- Name: logs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.logs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    action character varying(100) NOT NULL,
    target_agent_id uuid,
    detail jsonb,
    ip_address inet,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.logs OWNER TO postgres;

--
-- Name: permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.permissions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    code character varying(100) NOT NULL,
    description character varying(255),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.permissions OWNER TO postgres;

--
-- Name: role_permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.role_permissions (
    role_id uuid NOT NULL,
    permission_id uuid NOT NULL
);


ALTER TABLE public.role_permissions OWNER TO postgres;

--
-- Name: roles; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.roles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(50) NOT NULL,
    description character varying(255),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.roles OWNER TO postgres;

--
-- Name: rooms; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.rooms (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(255),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.rooms OWNER TO postgres;

--
-- Name: tokens; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_by uuid,
    token_hash character(64) NOT NULL,
    max_use integer,
    used_count integer DEFAULT 0 NOT NULL,
    expires_at timestamp with time zone,
    is_revoked boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tokens_max_use_check CHECK (((max_use IS NULL) OR (max_use > 0))),
    CONSTRAINT tokens_used_count_check CHECK ((used_count >= 0))
);


ALTER TABLE public.tokens OWNER TO postgres;

--
-- Name: user_sessions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.user_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash character(64) NOT NULL,
    ip_address inet,
    user_agent text,
    expires_at timestamp with time zone NOT NULL,
    last_activity_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone,
    revoked_reason character varying(255),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.user_sessions OWNER TO postgres;

--
-- Name: users; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    username character varying(50) NOT NULL,
    email character varying(255),
    password_hash character varying(255) NOT NULL,
    display_name character varying(100),
    role_id uuid NOT NULL,
    status character varying(20) DEFAULT 'ACTIVE'::character varying NOT NULL,
    failed_login_attempts integer DEFAULT 0 NOT NULL,
    locked_until timestamp with time zone,
    last_login_at timestamp with time zone,
    password_changed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT users_status_check CHECK (((status)::text = ANY ((ARRAY['ACTIVE'::character varying, 'DISABLED'::character varying, 'LOCKED'::character varying])::text[])))
);


ALTER TABLE public.users OWNER TO postgres;



--
-- Data for Name: permissions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.permissions (id, code, description, created_at) FROM stdin;
d436f316-45b5-4cce-ad7e-0f3556abaead	agents.manage	จัดการเครื่องลูก	2026-08-09 17:09:23.735729+07
b757aed8-3a55-4881-978b-7286ed747181	rooms.manage	จัดการห้องเรียน	2026-08-09 17:05:40.147227+07
6df6bc0f-78ab-4afb-b982-7f950c6ef1c8	tokens.manage	จัดการโทเคน	2026-08-09 17:06:28.763402+07
37ced93c-ca3e-47dc-a371-d77740449781	agents.read	อ่านรายละเอียดเครื่องลูก	2026-08-09 17:07:17.49964+07
70b5436e-dff2-45d8-8a72-70cb4b1aceb9	agents.control	ควบคุมเครื่องลูก	2026-08-09 17:07:38.402528+07
4de4c4b7-5e94-457b-bb74-b5fbd4e3c29f	agents.delete	ลบเครื่องลูก	2026-08-09 17:08:00.160735+07
2372a87c-4d1d-4f11-b6b0-1d0997a1b3be	agents.edit	แก้ไขเครื่องลูก	2026-08-09 17:08:18.873803+07
d5a70fc1-1fb4-4c40-a76c-e8e432097565	monitor.read	ดูจอเครื่องลูก	2026-08-09 17:10:30.862908+07
9906bc12-c615-44e4-91f6-219177e16acf	files.distribute	กระจายไฟล์	2026-08-09 17:12:12.68572+07
72f27308-7872-4a89-87a4-3dbf1f85890e	files.upload	อัปโหลดไฟล์สู่เซิฟเวอรื	2026-08-09 17:12:49.91707+07
b874f0d4-989f-4406-aa56-352b062fbab3	files.manage	จัดการไฟล์ภายในเซิฟเวอร์	2026-08-09 17:06:48.564466+07
75723302-69e7-4e17-8dcc-279747af9682	files.delete	ลบไฟล์บนเซิฟเวอร์	2026-08-09 17:13:02.464355+07
0fe25951-7eec-44e1-816d-b45a74766ad9	users.manage	จัดการผู้ใช้ในระบบ	2026-08-09 17:14:17.736601+07
97e59c9d-601d-4f89-b7b4-a2f508b418ac	logs.read	อ่านข้อมูล log ภายในระบบ	2026-08-09 17:14:33.712439+07
5fd8bbc1-9157-4c3b-9240-748ae525ae71	av.scan	สั่งแสกนไวรัส	2026-09-08 21:51:33.102388+07
59e38406-c55c-49e1-8d1d-9a2beb1d0f04	av.read	อ่านผลแสกนไวรัส	2026-09-08 21:51:42.545021+07
4528f9c1-8529-4dc0-9a49-338ff8212fcd	role.manage	จัดการยศ	2026-09-08 21:58:10.647729+07
\.


--
-- Data for Name: role_permissions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.role_permissions (role_id, permission_id) FROM stdin;
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	d436f316-45b5-4cce-ad7e-0f3556abaead
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	5fd8bbc1-9157-4c3b-9240-748ae525ae71
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	2372a87c-4d1d-4f11-b6b0-1d0997a1b3be
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	75723302-69e7-4e17-8dcc-279747af9682
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	b874f0d4-989f-4406-aa56-352b062fbab3
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	4528f9c1-8529-4dc0-9a49-338ff8212fcd
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	6df6bc0f-78ab-4afb-b982-7f950c6ef1c8
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	0fe25951-7eec-44e1-816d-b45a74766ad9
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	4de4c4b7-5e94-457b-bb74-b5fbd4e3c29f
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	37ced93c-ca3e-47dc-a371-d77740449781
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	59e38406-c55c-49e1-8d1d-9a2beb1d0f04
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	9906bc12-c615-44e4-91f6-219177e16acf
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	72f27308-7872-4a89-87a4-3dbf1f85890e
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	97e59c9d-601d-4f89-b7b4-a2f508b418ac
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	d5a70fc1-1fb4-4c40-a76c-e8e432097565
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	70b5436e-dff2-45d8-8a72-70cb4b1aceb9
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	b757aed8-3a55-4881-978b-7286ed747181
\.


--
-- Data for Name: roles; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.roles (id, name, description, created_at, updated_at) FROM stdin;
3fc98c0b-28f3-4286-88cf-20d50f6911f2	VIEWER	ผู้ชม	2026-08-11 15:19:10.806118+07	2026-08-11 15:19:10.806118+07
be808ed9-e820-46a8-a0d9-b3d1dd2defa1	ADMINISTRATOR	ผู้ดูแลระบบทั้งหมด	2026-08-07 15:52:23.246044+07	2026-09-14 15:08:23.695184+07
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.users (id, username, email, password_hash, display_name, role_id, status, failed_login_attempts, locked_until, last_login_at, password_changed_at, created_at, updated_at) FROM stdin;
23fc546a-5434-42b1-b811-15bc19986e0f	admin	johs@example.com	$2a$10$q2Zjt4nMlq8tY1trMefwnOpohEwAVWjT7dTafZjuCQaynD5ZlF5u.	John Does	be808ed9-e820-46a8-a0d9-b3d1dd2defa1	ACTIVE	0	\N	2026-09-14 20:00:47.876749+07	\N	2026-08-09 17:15:21.749211+07	2026-08-09 17:15:21.749211+07
\.


--
-- Name: agents agents_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);


--
-- Name: av_jobs av_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.av_jobs
    ADD CONSTRAINT av_jobs_pkey PRIMARY KEY (id);


--
-- Name: av_scan_results av_scan_results_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.av_scan_results
    ADD CONSTRAINT av_scan_results_pkey PRIMARY KEY (id);


--
-- Name: commands commands_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.commands
    ADD CONSTRAINT commands_pkey PRIMARY KEY (id);


--
-- Name: file_distribution_jobs file_distribution_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_distribution_jobs
    ADD CONSTRAINT file_distribution_jobs_pkey PRIMARY KEY (id);


--
-- Name: file_distribution_jobs file_distribution_jobs_request_unique; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_distribution_jobs
    ADD CONSTRAINT file_distribution_jobs_request_unique UNIQUE (requested_by, request_id);


--
-- Name: file_distribution_targets file_distribution_targets_job_agent_unique; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_distribution_targets
    ADD CONSTRAINT file_distribution_targets_job_agent_unique UNIQUE (job_id, agent_id);


--
-- Name: file_distribution_targets file_distribution_targets_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_distribution_targets
    ADD CONSTRAINT file_distribution_targets_pkey PRIMARY KEY (id);


--
-- Name: file_download_grants file_download_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_download_grants
    ADD CONSTRAINT file_download_grants_pkey PRIMARY KEY (id);


--
-- Name: file_download_grants file_download_grants_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_download_grants
    ADD CONSTRAINT file_download_grants_token_hash_key UNIQUE (token_hash);


--
-- Name: files files_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.files
    ADD CONSTRAINT files_pkey PRIMARY KEY (id);


--
-- Name: logs logs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.logs
    ADD CONSTRAINT logs_pkey PRIMARY KEY (id);


--
-- Name: permissions permissions_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_code_key UNIQUE (code);


--
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: roles roles_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_name_key UNIQUE (name);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: rooms rooms_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rooms
    ADD CONSTRAINT rooms_name_key UNIQUE (name);


--
-- Name: rooms rooms_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rooms
    ADD CONSTRAINT rooms_pkey PRIMARY KEY (id);


--
-- Name: tokens tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT tokens_pkey PRIMARY KEY (id);


--
-- Name: tokens tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: av_scan_results uq_av_scan_command; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.av_scan_results
    ADD CONSTRAINT uq_av_scan_command UNIQUE (command_id);


--
-- Name: av_scan_results uq_av_scan_job_agent; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.av_scan_results
    ADD CONSTRAINT uq_av_scan_job_agent UNIQUE (job_id, agent_id);


--
-- Name: user_sessions user_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (id);


--
-- Name: user_sessions user_sessions_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_token_hash_key UNIQUE (token_hash);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: idx_agents_last_seen; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_agents_last_seen ON public.agents USING btree (last_seen);


--
-- Name: idx_agents_room_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_agents_room_id ON public.agents USING btree (room_id);


--
-- Name: idx_agents_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_agents_status ON public.agents USING btree (status);


--
-- Name: idx_av_jobs_requested_by_created_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_av_jobs_requested_by_created_at ON public.av_jobs USING btree (requested_by, created_at DESC);


--
-- Name: idx_av_scan_results_agent_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_av_scan_results_agent_id ON public.av_scan_results USING btree (agent_id);


--
-- Name: idx_av_scan_results_command_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_av_scan_results_command_id ON public.av_scan_results USING btree (command_id);


--
-- Name: idx_commands_agent_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_commands_agent_id ON public.commands USING btree (agent_id);


--
-- Name: idx_commands_created_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_commands_created_at ON public.commands USING btree (created_at DESC);


--
-- Name: idx_commands_issued_by; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_commands_issued_by ON public.commands USING btree (issued_by);


--
-- Name: idx_commands_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_commands_status ON public.commands USING btree (status);


--
-- Name: idx_distribution_jobs_status_created; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_distribution_jobs_status_created ON public.file_distribution_jobs USING btree (status, created_at DESC);


--
-- Name: idx_distribution_targets_agent; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_distribution_targets_agent ON public.file_distribution_targets USING btree (agent_id);


--
-- Name: idx_distribution_targets_job_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_distribution_targets_job_status ON public.file_distribution_targets USING btree (job_id, status);


--
-- Name: idx_download_grants_expires; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_download_grants_expires ON public.file_download_grants USING btree (expires_at);


--
-- Name: idx_files_uploaded_by; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_files_uploaded_by ON public.files USING btree (uploaded_by);


--
-- Name: idx_logs_agent_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_logs_agent_id ON public.logs USING btree (target_agent_id);


--
-- Name: idx_logs_created_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_logs_created_at ON public.logs USING btree (created_at DESC);


--
-- Name: idx_logs_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_logs_user_id ON public.logs USING btree (user_id);


--
-- Name: idx_role_permissions_permission_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_role_permissions_permission_id ON public.role_permissions USING btree (permission_id);


--
-- Name: idx_tokens_created_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_tokens_created_at ON public.tokens USING btree (created_at DESC, id DESC);


--
-- Name: idx_tokens_created_by; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_tokens_created_by ON public.tokens USING btree (created_by);


--
-- Name: idx_user_sessions_expires_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_sessions_expires_at ON public.user_sessions USING btree (expires_at);


--
-- Name: idx_user_sessions_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_sessions_user_id ON public.user_sessions USING btree (user_id);


--
-- Name: idx_users_role_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_role_id ON public.users USING btree (role_id);


--
-- Name: idx_users_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_status ON public.users USING btree (status);


--
-- Name: av_jobs av_jobs_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.av_jobs
    ADD CONSTRAINT av_jobs_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: av_scan_results av_scan_results_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.av_scan_results
    ADD CONSTRAINT av_scan_results_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.av_jobs(id) ON DELETE RESTRICT;


--
-- Name: file_distribution_jobs file_distribution_jobs_file_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_distribution_jobs
    ADD CONSTRAINT file_distribution_jobs_file_id_fkey FOREIGN KEY (file_id) REFERENCES public.files(id) ON DELETE RESTRICT;


--
-- Name: file_distribution_jobs file_distribution_jobs_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_distribution_jobs
    ADD CONSTRAINT file_distribution_jobs_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: file_distribution_jobs file_distribution_jobs_room_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_distribution_jobs
    ADD CONSTRAINT file_distribution_jobs_room_id_fkey FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE RESTRICT;


--
-- Name: file_distribution_targets file_distribution_targets_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_distribution_targets
    ADD CONSTRAINT file_distribution_targets_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE RESTRICT;


--
-- Name: file_distribution_targets file_distribution_targets_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_distribution_targets
    ADD CONSTRAINT file_distribution_targets_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.file_distribution_jobs(id) ON DELETE CASCADE;


--
-- Name: file_download_grants file_download_grants_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_download_grants
    ADD CONSTRAINT file_download_grants_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: file_download_grants file_download_grants_file_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_download_grants
    ADD CONSTRAINT file_download_grants_file_id_fkey FOREIGN KEY (file_id) REFERENCES public.files(id) ON DELETE CASCADE;


--
-- Name: file_download_grants file_download_grants_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.file_download_grants
    ADD CONSTRAINT file_download_grants_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.file_distribution_jobs(id) ON DELETE CASCADE;


--
-- Name: agents fk_agents_room; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_room FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE SET NULL;


--
-- Name: av_scan_results fk_av_scan_agent; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.av_scan_results
    ADD CONSTRAINT fk_av_scan_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: av_scan_results fk_av_scan_command; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.av_scan_results
    ADD CONSTRAINT fk_av_scan_command FOREIGN KEY (command_id) REFERENCES public.commands(id) ON DELETE CASCADE;


--
-- Name: commands fk_commands_agent; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.commands
    ADD CONSTRAINT fk_commands_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: commands fk_commands_user; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.commands
    ADD CONSTRAINT fk_commands_user FOREIGN KEY (issued_by) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: files fk_files_uploaded_by; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.files
    ADD CONSTRAINT fk_files_uploaded_by FOREIGN KEY (uploaded_by) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: logs fk_logs_agent; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.logs
    ADD CONSTRAINT fk_logs_agent FOREIGN KEY (target_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;


--
-- Name: logs fk_logs_user; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.logs
    ADD CONSTRAINT fk_logs_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: role_permissions fk_role_permissions_permission; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT fk_role_permissions_permission FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE;


--
-- Name: role_permissions fk_role_permissions_role; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT fk_role_permissions_role FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- Name: user_sessions fk_user_sessions_user; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT fk_user_sessions_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: users fk_users_role; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT fk_users_role FOREIGN KEY (role_id) REFERENCES public.roles(id);


--
-- Name: tokens tokens_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT tokens_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- PostgreSQL database dump complete
--

\unrestrict uJ2GW4CG8nNogm38dcFhLKatnF01weITrJ2GH0VNNu4aLAmzXist1iX0LcokZ1c

