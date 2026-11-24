# SQLite schema

[`sqlite/LATEST.sql`](sqlite/LATEST.sql) initializes a new Cat_two database. The running app checks that an existing database matches the current schema before it starts serving requests. Incompatible databases are rejected without applying migrations.

Keep existing data volumes with their original image. Start the new release with a separate volume or empty directory. [Deployment and backup](../../.agents/skills/cat-two-product-development/references/deployment.md#备份和升级)
