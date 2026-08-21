# Git Deployment & Private Repositories

LITEPLOY supports deploying directly from any Git provider, including GitHub, GitLab, Gitea, Bitbucket, or self-hosted Git servers.

---

## 🔑 Authentication Methods

### 1. Public Repositories (No Credentials)
Select **Public Repository (No Credentials)** for open-source or public repositories.

### 2. Personal Access Tokens (PAT)
Required for private HTTPS repository URLs (`https://github.com/org/repo.git`):
1. Select **Personal Access Token (PAT)** in Authentication Type.
2. Enter your PAT token (`ghp_...` for GitHub or `glpat-...` for GitLab).
3. **Secret Masking:** LITEPLOY masks all tokens in UI inputs and automatically redacts tokens (`******`) from build outputs.

### 3. SSH Deploy Keys
Required for SSH repository URLs (`git@github.com:org/repo.git`):
1. Generate an SSH keypair on your local machine:
   ```bash
   ssh-keygen -t ed25519 -C "liteploy-deploy-key" -f id_liteploy
   ```
2. Add the **Public Key** (`id_liteploy.pub`) to your repository's **Deploy Keys** settings on GitHub/GitLab.
3. Paste the **Private Key** (`id_liteploy`) into LITEPLOY's **SSH Private Key** input.
4. LITEPLOY writes the key to a temporary file (`0600` permissions) during clone operations and removes it immediately afterwards.
