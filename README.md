# dbchan

A Kubernetes operator for managing database resources including datastores, databases, users, and migrations across multiple database engines (MySQL/MariaDB, PostgreSQL, SQL Server).

## Description

dbchan is a comprehensive Kubernetes operator that provides declarative management of database resources. It allows you to manage database servers, logical databases, users, and schema migrations through Kubernetes Custom Resource Definitions (CRDs), supporting multiple database engines with a unified interface.

### Key Features

- **Multi-Database Support**: MySQL/MariaDB, PostgreSQL, SQL Server
- **Declarative Management**: Define database resources as Kubernetes resources
- **Migration Management**: Version-controlled SQL migrations with rollback support
- **User Management**: Database user and role management
- **Secret Integration**: Secure credential management using Kubernetes Secrets
- **Status Tracking**: Comprehensive status reporting for all resources
- **Finalizer Support**: Proper cleanup of external resources

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/dbchan:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/dbchan:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/dbchan:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/dbchan/<tag or branch>/dist/install.yaml
```

## Custom Resource Definitions (CRDs)

dbchan provides four main CRDs for comprehensive database management:

### 1. Datastore

The `Datastore` CRD represents a database server instance and manages the connection to external database servers.

#### Specification

```yaml
apiVersion: db.fatiudeen.dev/v1
kind: Datastore
metadata:
  name: my-postgres-server  # This becomes the datastore name
spec:
  datastoreType: "postgres"  # mysql, mariadb, postgres, postgresql, sqlserver, mssql
  secretRef:
    name: "postgres-credentials"
    # Optional: specify custom secret keys (defaults shown)
    usernameKey: "username"     # default: "username"
    passwordKey: "password"     # default: "password"
    hostKey: "host"             # default: "host"
    portKey: "port"             # default: "port"
    sslModeKey: "sslmode"       # default: "sslmode" (PostgreSQL only)
    instanceKey: "instance"     # default: "instance" (SQL Server only)
```

#### Status

```yaml
status:
  phase: "Ready"           # Connecting, Ready, Failed
  ready: true              # boolean indicating connection status
  message: "Connected successfully"
```

#### Example Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-credentials
type: Opaque
data:
  username: cG9zdGdyZXM=  # base64 encoded
  password: cGFzc3dvcmQ=  # base64 encoded
  host: bG9jYWxob3N0     # base64 encoded
  port: NTQzMg==         # base64 encoded (5432)
  sslmode: ZGlzYWJsZQ==  # base64 encoded (disable)
```

#### Usage

1. **Create a secret** with database credentials
2. **Create a Datastore** resource pointing to the secret
3. **Monitor status** - the controller will attempt to connect and update the status

### 2. Database

The `Database` CRD represents a logical database within a datastore.

#### Specification

```yaml
apiVersion: db.fatiudeen.dev/v1
kind: Database
metadata:
  name: myapp-db  # This becomes the database name
spec:
  datastoreRef:
    name: "my-postgres-server"  # references the datastore
  charset: "utf8mb4"            # optional, database-specific
  collation: "utf8mb4_unicode_ci"  # optional, database-specific
```

#### Status

```yaml
status:
  phase: "Ready"           # Creating, Ready, Failed
  ready: true              # boolean indicating creation status
  message: "Database created successfully"
  createdAt: "2025-01-27T10:30:00Z"
```

#### Database-Specific Options

**MySQL/MariaDB:**
```yaml
spec:
  charset: "utf8mb4"
  collation: "utf8mb4_unicode_ci"
```

**PostgreSQL:**
```yaml
spec:
  charset: "UTF8"
  collation: "en_US.UTF-8"
```

**SQL Server:**
```yaml
spec:
  collation: "SQL_Latin1_General_CP1_CI_AS"
```

#### Usage

1. **Ensure Datastore is Ready** - Database creation waits for datastore to be ready
2. **Create Database** resource with datastore reference
3. **Monitor status** - controller creates the logical database

### 3. User

The `User` CRD represents a database user or role.

#### Specification

```yaml
apiVersion: db.fatiudeen.dev/v1
kind: User
metadata:
  name: app-user
spec:
  username: "app_user"
  password: "cGFzc3dvcmQxMjM="  # base64 encoded password
  datastoreRef:
    name: "my-postgres-server"  # references the datastore
  databaseRef:                  # optional, for database-specific users
    name: "myapp-db"
  roles:                        # optional, database-specific roles
    - "readwrite"
  privileges:                   # optional, specific privileges
    - "SELECT"
    - "INSERT"
    - "UPDATE"
  host: "localhost"             # optional, for MySQL/MariaDB
```

#### Status

```yaml
status:
  phase: "Ready"           # Creating, Ready, Failed
  ready: true              # boolean indicating creation status
  created: true            # boolean indicating user exists
  message: "User created successfully"
  createdAt: "2025-01-27T10:30:00Z"
```

#### Database-Specific Behavior

**MySQL/MariaDB:**
- Creates user with host specification
- Grants privileges to specific database if `databaseRef` provided
- Supports roles and privileges

**PostgreSQL:**
- Creates role/user
- Can be database-specific or global
- Supports role inheritance

**SQL Server:**
- Creates login and user
- Maps to specific database if `databaseRef` provided

#### Usage

1. **Base64 encode password**: `echo -n "password123" | base64`
2. **Create User** resource with datastore reference
3. **Optional**: Specify database reference for database-specific users
4. **Monitor status** - controller creates the database user

### 4. Migration

The `Migration` CRD represents a one-time SQL script with version control and rollback support.

#### Specification

```yaml
apiVersion: db.fatiudeen.dev/v1
kind: Migration
metadata:
  name: migration-001-create-users-table
spec:
  version: "001"                    # unique version identifier
  description: "Create users table" # optional description
  databaseRef:
    name: "myapp-db"               # references the database
  sql: |
    CREATE TABLE users (
      id INT AUTO_INCREMENT PRIMARY KEY,
      username VARCHAR(255) NOT NULL UNIQUE,
      email VARCHAR(255) NOT NULL UNIQUE,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
  rollbackSql: |                   # optional rollback SQL
    DROP TABLE IF EXISTS users;
```

#### Status

```yaml
status:
  phase: "Applied"           # Pending, Applied, Failed
  applied: true              # boolean indicating if migration was applied
  appliedAt: "2025-01-27T10:30:00Z"
  message: "Migration applied successfully"
  checksum: "a1b2c3d4e5f6..."  # SHA256 checksum of SQL content
```

#### Migration Tracking

The controller automatically creates a `schema_migrations` table to track applied migrations:

```sql
CREATE TABLE schema_migrations (
  version VARCHAR(255) PRIMARY KEY,
  applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  checksum VARCHAR(64)
);
```

#### Usage

1. **Create Migration** resource with version and SQL
2. **Optional**: Provide rollback SQL for safe deletion
3. **Monitor status** - controller applies migration once
4. **Delete Migration** - executes rollback SQL if provided

## Complete Example

Here's a complete example of setting up a PostgreSQL database with a user and migration:

### 1. Create Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-credentials
type: Opaque
data:
  username: cG9zdGdyZXM=
  password: cGFzc3dvcmQxMjM=
  host: bG9jYWxob3N0
  port: NTQzMg==
  sslmode: ZGlzYWJsZQ==
```

### 2. Create Datastore

```yaml
apiVersion: db.fatiudeen.dev/v1
kind: Datastore
metadata:
  name: postgres-server
spec:
  datastoreType: "postgres"
  secretRef:
    name: "postgres-credentials"
```

### 3. Create Database

```yaml
apiVersion: db.fatiudeen.dev/v1
kind: Database
metadata:
  name: myapp-db
spec:
  datastoreRef:
    name: "postgres-server"
```

### 4. Create User

```yaml
apiVersion: db.fatiudeen.dev/v1
kind: User
metadata:
  name: app-user
spec:
  username: "app_user"
  password: "cGFzc3dvcmQxMjM="
  datastoreRef:
    name: "postgres-server"
  databaseRef:
    name: "myapp-db"
```

### 5. Create Migration

```yaml
apiVersion: db.fatiudeen.dev/v1
kind: Migration
metadata:
  name: migration-001-create-users-table
spec:
  version: "001"
  description: "Create users table"
  databaseRef:
    name: "myapp-db"
  sql: |
    CREATE TABLE users (
      id SERIAL PRIMARY KEY,
      username VARCHAR(255) NOT NULL UNIQUE,
      email VARCHAR(255) NOT NULL UNIQUE,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
  rollbackSql: |
    DROP TABLE IF EXISTS users;
```

## Resource Dependencies

The CRDs have the following dependency chain:

```
Secret → Datastore → Database → User/Migration
```

1. **Secret** contains database credentials
2. **Datastore** references secret and validates connection
3. **Database** references datastore and creates logical database
4. **User/Migration** reference database and perform operations

## Status Monitoring

All resources provide comprehensive status information:

```bash
# Check datastore status
kubectl get datastores

# Check database status
kubectl get databases

# Check user status
kubectl get users

# Check migration status
kubectl get migrations

# Get detailed status
kubectl describe datastore postgres-server
kubectl describe database myapp-db
kubectl describe user app-user
kubectl describe migration migration-001-create-users-table
```

## Troubleshooting

### Common Issues

1. **Datastore Connection Failed**
   - Check secret credentials
   - Verify network connectivity
   - Check datastore type spelling

2. **Database Creation Failed**
   - Ensure datastore is ready
   - Check database name validity
   - Verify charset/collation values

3. **User Creation Failed**
   - Ensure database is ready
   - Check password encoding (must be base64)
   - Verify username format

4. **Migration Failed**
   - Check SQL syntax
   - Ensure database is ready
   - Verify version uniqueness

### Debugging

```bash
# Check controller logs
kubectl logs -n dbchan-system deployment/dbchan-controller-manager

# Check resource events
kubectl get events --sort-by=.metadata.creationTimestamp

# Check resource status
kubectl get <resource> -o yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

