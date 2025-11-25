create database if not exists device_discovery
character set utf8mb4
collate utf8mb4_unicode_ci;

use device_discovery;

insert into discovery_rules (
    name,
    enabled,
    ranges_json,
    host_timeout_secs,
    concurrency
)values (
            '测试规则1',
            1,
            '[{"StartIP":"192.168.1.1","EndIP":"192.168.1.5"}]',
            2,
            32
        );

insert into discovery_schedules(
    rule_id,type,expression,timezone,
    enable,overlap_policy,incremental,
    full_every_n_runs,host_timeout_secs,concurrency
)values(
           1,
           'cron',
           '* * * * *',
           'UTC',
           1,
           'queue',
           0,
           NULL,
           2,
           32
       );

-- select * from job_runs order by id desc limit 10;

-- select * from device_models order by last_seen desc limit 20;
