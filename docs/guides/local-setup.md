# Run the platform with Compose

For host development and the local login account, start with the [quick start](../../README.md#run-locally).
Use Compose to run the infrastructure alone or the full platform:

```sh
docker compose up --build                         # PostgreSQL, Keycloak, registry
docker compose --profile orkest up --build        # also build and run Orkest
```

The `orkest` profile exposes port 8080 and mounts the host Docker socket so the
platform can launch independent worker containers. Build and publish the
[runtime images](../reference/executors/docker.md#build-and-configure-images)
before task execution. Compose connects workers through the shared Docker network.

Set `ORKEST_DATA_DIR` to a path visible to the Docker daemon: worker containers
bind-mount durable storage from the daemon's host, even when Orkest runs in a
container. For named Docker contexts and separate browser/backend OIDC addresses,
see [deployment configuration](../operations/configuration.md).

`make db-down` retains the PostgreSQL volume. Changing initialization variables
does not update an existing database. Delete the volume only when deleting its
data is intentional.
