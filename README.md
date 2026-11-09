# TODOs

- [ ] The working dir needs to be an arg to these scripts, we can control it in the .env file or something outside of them. They can use their own internal .env file too
- [ ] Make runner dir just use the TYPE field of the runner we don't care about HOST like this
- [ ] Found a bug where if you attempt to remove runners while they are running, the scripts break midway through. Probably better to separate remove services from the remove runner files. Or just let the runner config script handle the service removal
- [ ] "Enter username for account" is vague when you come back to this after time. Account refers to the account to run actions with, specify that in the statement
