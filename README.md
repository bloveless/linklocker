# LinkLocker
Link storage app

# Local Development

If you've got Google Chrome installed locally then you need only to provide some environment variables to start the application.

1. Copy `.env.example` to `.env`
2. Generate some fairly long strings to put in `SESSION_SECERT` and `CSRF_SECERT`. I used 40 character random strings.
3. You'll now be able to run `go run .../.` to start serving LinkLocker on port 3000.

If you don't have Google Chrome installed and you'd like to run Google Chrome headless in a docker container to generate
your screenshots then follow these additional instructions.

1. Edit your `.env` file from above and add a line for `CHROME_URL=ws://localhost:9222` this will tell LinkLocker to use a remote version of Google Chrome to create the screenshots and in this case it will use the Google Chrome headless docker image
2. You can run `make up` instead of `go run .../.` which will start the docker container for you.
