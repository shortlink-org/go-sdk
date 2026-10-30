alter table events drop column type;
alter table events alter column created_at drop default;
alter table events alter column id drop default;

alter table aggregates alter column updated_at drop default;
alter table aggregates alter column created_at drop default;

alter table snapshots alter column updated_at drop default;
alter table snapshots alter column created_at drop default;
