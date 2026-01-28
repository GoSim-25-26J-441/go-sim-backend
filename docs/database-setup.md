# Database Setup Guide

## Running Migrations with psql on Windows

### Method 1: Using psql Command Line (Recommended)

1. **Open PowerShell or Command Prompt**

2. **Navigate to project directory:**
   ```powershell
   cd C:\Users\Manilka\Desktop\Manilka\final_research\go-sim-backend
   ```

3. **Run the migration:**
   ```powershell
   psql -h localhost -p 5432 -U postgres -d gosim -f migrations/0001_auth_users.sql
   ```

   You will be prompted for the PostgreSQL password.

4. **Using environment variables:**
   ```powershell
   $env:PGPASSWORD = "your_password"
   psql -h localhost -p 5432 -U postgres -d gosim -f migrations/0001_auth_users.sql
   ```

### Method 2: Using PowerShell Script

We've provided a PowerShell script for convenience:

```powershell
.\scripts\run-migration.ps1
```

Or specify a different migration file:
```powershell
.\scripts\run-migration.ps1 migrations/0001_auth_users.sql
```

### Method 3: Using psql Interactive Shell

1. **Open psql shell:**
   ```powershell
   psql -h localhost -p 5432 -U postgres -d gosim
   ```

2. **Once connected, run:**
   ```sql
   \i migrations/0001_auth_users.sql
   ```

   Note: Use forward slashes `/` or full Windows path like `C:/path/to/file.sql`

3. **Or paste the SQL directly:**
   Copy the contents of `migrations/0001_auth_users.sql` and paste into the psql shell.

### Method 4: Using pgAdmin (GUI)

1. Open pgAdmin
2. Connect to your PostgreSQL server
3. Right-click on your database (`gosim`)
4. Select "Query Tool"
5. Open the file `migrations/0001_auth_users.sql`
6. Click "Execute" (F5)

## Connection Parameters

Based on your `.env.example`:

- **Host:** `localhost`
- **Port:** `5432`
- **User:** `postgres`
- **Database:** `gosim`
- **Password:** (from your `.env` file or PostgreSQL setup)

## Verifying Migration

After running the migration, verify it worked:

```sql
-- Connect to database
psql -h localhost -p 5432 -U postgres -d gosim

-- Check if table exists
\dt users

-- Check table structure
\d users

-- Check if trigger exists
SELECT trigger_name, event_manipulation, event_object_table
FROM information_schema.triggers
WHERE event_object_table = 'users';
```

## Troubleshooting

### "psql: command not found"
- Make sure PostgreSQL bin directory is in your PATH
- Or use full path: `C:\Program Files\PostgreSQL\18\bin\psql.exe`

### "password authentication failed"
- Check your `.env` file for `DB_PASSWORD`
- Or use: `$env:PGPASSWORD = "your_password"`

### "database does not exist"
- Create the database first:
  ```sql
  CREATE DATABASE gosim;
  ```

### "permission denied"
- Make sure you're using a user with sufficient privileges (usually `postgres` superuser)

