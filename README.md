# Axion - Declarative Configuration Management

**Axion** is a modern configuration management solution that enforces desired system state through declarative manifests. Built for reliability and performance, it separates orchestration logic from execution to provide a clean, robust architecture for infrastructure as code.

## 🎬 Live Demo

The following demonstrations show the core `plan` and `apply` workflow.

![Axion Demo](/.github/assets/recording.gif)

## Why Axion?

### 🚀 Fast & Lightweight
- **Single Go binaries** with no runtime dependencies
- **High performance** execution with minimal resource overhead
- **Quick deployment** - drop the binary and run

### 🛡️ Safe & Reliable Operations
- **Plan-before-apply workflow** - safely preview all intended changes before execution
- **Atomic rollbacks** - if any resource fails during apply, automatically rolls back all changes from that run
- **Built-in backups** - automatically creates backups of files and directories before modification
- **Optimistic concurrency control** - uses ETags for safe concurrent API operations

### 🔄 Smart State Management
- **Idempotent execution** - only changes what's necessary through state drift detection
- **Current state validation** - checks actual system state before taking action
- **Minimal disruption** - only performs operations when system has drifted from desired state

### 📊 Intelligent Dependency Management
- **Automatic DAG construction** - builds dependency graphs to ensure correct execution order
- **Complex setup simplification** - handles intricate resource relationships automatically
- **Parallel execution** where dependencies allow for faster operations

### 🎯 Flexible Configuration Options
- **Multiple manifest formats** - choose between simple YAML or powerful Starlark scripting
- **Variable templating** in YAML for reusability
- **Dynamic configuration** with Starlark's programming constructs (loops, functions, logic)
- **Extensible architecture** - easily add new resource types and capabilities

## Common Use Cases

- **System Configuration**: Manage config files, users, and system settings
- **Application Deployment**: Deploy and configure applications with dependencies
- **Infrastructure Setup**: Bootstrap servers and install required packages
- **Compliance Management**: Ensure systems maintain required security postures

## Architecture Overview

Axion's client/server architecture separates concerns for clarity and security.

* **axiond (Agent):** A minimal, resource-oriented REST API server that runs on target nodes. It is responsible for the "how"—executing low-level system commands to manage files, directories, etc.
* **axionctl (CLI):** The "brains" of the operation. It is responsible for the "what"—parsing manifests, building the dependency graph, planning the execution and calling the `axiond` API in the correct sequence.

## Manifest Formats

Axion gives you the flexibility to choose the right tool for the job.

* **YAML:** Ideal for simple, static and easily readable configurations. It supports variable templating for reusability.
* **Starlark:** A dialect of Python, perfect for when you need logic, loops, functions or other programming constructs to generate your resource definitions dynamically.

## Creating a Manifest File

### YAML 

Create a YAML manifest file (e.g., deployment.yaml) to define your desired configuration.

Note: The entire file is parsed before processing, which means you can define a resource (a) that depends on another resource (b) before b appears in the file. Axion resolves all dependencies by their string id after parsing the whole document.

```yaml
variables:
  default_owner: marcel
  default_group: wheel

resources:
  - id: a
    type: file
    state: present
    properties:
      path: /tmp/foo/a.txt
      mode: "0600"
      owner: "{{ .default_owner }}"
    dependencies:
      - b # /tmp/foo directory needs to exists...

  - id: b
    type: directory
    state: present
    properties:
      path: /tmp/foo
      mode: "0755"
  
  - id: c
    type: file
    state: present
    properties:
      path: /tmp/bar.txt
      mode: "0600"
      owner: "{{ .default_owner }}"
```

### Starlark 

Create a Starlark manifest file (e.g., deployment.star) to define your desired configuration.

Note: Starlark is a scripting language that executes from top to bottom. Therefore, a resource variable (like b) must be defined before it can be passed as a reference into another resource's dependencies list.

```python
default_owner = "marcel"
default_group = "wheel"

b = resources.directory(
    state = "present",
    path  = "/tmp/foo",
    mode  = "0755"
)

a = resources.file(
    state        = "present",
    path         = "/tmp/foo/a.txt",
    mode         = "0600",
    owner        = default_owner,
    dependencies = [b]
)

c = resources.file(
    state       = "present",
    path        = "/tmp/bar.txt",
    mode        = "0600",
    owner       = default_owner
)
```