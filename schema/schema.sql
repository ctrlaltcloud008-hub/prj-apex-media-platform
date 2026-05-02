ALTER DATABASE `videopipeline-db` SET OPTIONS (
  version_retention_period = '7d'
);

CREATE TABLE SchemaMigrations (
  Version INT64 NOT NULL,
  Dirty BOOL NOT NULL,
) PRIMARY KEY(Version);
