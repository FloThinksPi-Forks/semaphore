alter table `project__repository` add column `git_tag` varchar(255) not null default '';
alter table `project__repository` add column `verify_signature` bool not null default false;
alter table `project__repository` add column `signing_key` varchar(8192) not null default '';
