--
-- MSSQL database schema for speedtest telemetry
--

CREATE TABLE [dbo].[speedtest_users](
    [id]        [bigint] IDENTITY(120,1) NOT NULL,
    [timestamp] [datetime] NOT NULL,
    [ip]        [nvarchar](max) NOT NULL,
    [ispinfo]   [nvarchar](max) NULL,
    [extra]     [nvarchar](max) NULL,
    [ua]        [nvarchar](max) NOT NULL,
    [lang]      [nvarchar](max) NOT NULL,
    [dl]        [nvarchar](max) NULL,
    [ul]        [nvarchar](max) NULL,
    [ping]      [nvarchar](max) NULL,
    [jitter]    [nvarchar](max) NULL,
    [log]       [nvarchar](max) NULL,
    [uuid]      [nvarchar](max) NULL,
    [grade_data]         [nvarchar](max) NULL,
    [chart_data]         [nvarchar](max) NULL,
    [latency_underload]  [nvarchar](max) NULL,
    [ping_during_test]   [nvarchar](max) NULL,
    [client_id]          [nvarchar](max) NULL,
 CONSTRAINT [PK_speedtest_users] PRIMARY KEY CLUSTERED
(
    [id] ASC
) WITH (PAD_INDEX = OFF, STATISTICS_NORECOMPUTE = OFF, IGNORE_DUP_KEY = OFF, ALLOW_ROW_LOCKS = ON, ALLOW_PAGE_LOCKS = ON) ON [PRIMARY]
) ON [PRIMARY] TEXTIMAGE_ON [PRIMARY]
GO

ALTER TABLE [dbo].[speedtest_users] ADD  CONSTRAINT [DF_speedtest_users_timestamp]  DEFAULT (getdate()) FOR [timestamp]
GO

-- Migration for existing installs: add the columns above if they are missing.
--
-- IF COL_LENGTH('dbo.speedtest_users', 'grade_data') IS NULL
--     ALTER TABLE [dbo].[speedtest_users] ADD [grade_data] [nvarchar](max) NULL
-- GO
-- IF COL_LENGTH('dbo.speedtest_users', 'chart_data') IS NULL
--     ALTER TABLE [dbo].[speedtest_users] ADD [chart_data] [nvarchar](max) NULL
-- GO
-- IF COL_LENGTH('dbo.speedtest_users', 'latency_underload') IS NULL
--     ALTER TABLE [dbo].[speedtest_users] ADD [latency_underload] [nvarchar](max) NULL
-- GO
-- IF COL_LENGTH('dbo.speedtest_users', 'ping_during_test') IS NULL
--     ALTER TABLE [dbo].[speedtest_users] ADD [ping_during_test] [nvarchar](max) NULL
-- GO
-- IF COL_LENGTH('dbo.speedtest_users', 'client_id') IS NULL
--     ALTER TABLE [dbo].[speedtest_users] ADD [client_id] [nvarchar](max) NULL
-- GO
