# User accounts
{ config, pkgs, lib, ... }:

{
  users.mutableUsers = false; # Declarative users only

  # Admin account (your SSH access)
  users.users.root.openssh.authorizedKeys.keys = [
    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDQFeDfovakWHp1alVhdI6qEI7Tw+/VLtbmFBRYGWwCCzGTvKl0TdG0KRdY4GJtDzXdOaYYwYobvCJ1w8Ww/MjKa1/FgA1XeUrwJrxdTXJE8gFK+sgtw/E4Qq+EJH3HBPAWLGuhMmQk7Sg1vx+mcQ1ejhaW2tM9KFM24tcRm4XCDZBoCZbmXCBBSjqM0KM+Zj5WH7qtb33JPHyIYdbvyKzVjklNeF9Sf9iLsAa1lpavqtRdQ+d/6TMK+u+fj9imsb4kIhOCXZTA9pyrp9HrIkSK4aCe1dACluTUmS8DYOAC9PM1STXah0WMFiG4IfoePCMus9VM/zA05FeH9ho6uG5n aleks.clark@gmail.com"
  ];

  # Student account - no sudo, no admin access
  users.users.student = {
    isNormalUser = true;
    description = "Student";
    home = "/home/student";
    shell = pkgs.bash;
    # No wheel group = no sudo
    extraGroups = [ "video" "render" ]; # GPU access for sway/ghostty
    # Set initial password (student can't change it without sudo)
    hashedPassword = "$6$rounds=10000$primer$placeholder"; # TODO: set real password
  };

  users.groups.students = {};
}
