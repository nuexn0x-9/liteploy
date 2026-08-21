# Persistent Volumes

LITEPLOY supports mounting host directories into application containers to persist data across deployments. 
By default, Docker containers are ephemeral. When you deploy a new version of an application, the old container is destroyed and replaced with a new one. Any data written inside the container (e.g., a SQLite database, user uploads, logs) is lost.

Persistent volumes map a directory from the VPS host filesystem directly into the container filesystem.

## How to Configure

1. Navigate to the **Application Detail** page.
2. Scroll to the **PERSISTENT VOLUMES** section.
3. Click `[ + ADD VOLUME ]`.
4. Enter the **Host Path** (e.g. `/var/lib/liteploy/volumes/myapp/data`). This is the path on the host VPS.
5. Enter the **Container Path** (e.g. `/app/data`). This is the path inside the container where the data is expected.
6. Click `[ SAVE VOLUMES ]`.
7. Click `▲ DEPLOY NOW` for the changes to take effect.

## Use Cases

- **Databases**: Deploying PostgreSQL or MySQL requires persisting `/var/lib/postgresql/data` or `/var/lib/mysql`.
- **Uploads**: Saving user uploaded files to the host disk.
- **Stateful applications**: Any application that maintains internal state as flat files.
