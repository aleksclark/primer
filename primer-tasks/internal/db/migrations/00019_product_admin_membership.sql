-- Product administrators use the existing parent membership/session authority
-- model, but are intentionally distinct from household administrators. They
-- administer global product catalogs rather than a tenant's household data.
ALTER TABLE parent_memberships DROP CONSTRAINT IF EXISTS parent_memberships_role_check;
ALTER TABLE parent_memberships
  ADD CONSTRAINT parent_memberships_role_check CHECK (role IN ('admin', 'product_admin'));
