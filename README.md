# ChangeMonitor
ChangeMonitor is an application for monitoring changes on websites. 

The following features are supported:
* Chrome support – detect changes on pages using Javascript.
* Support for JSON and CSS selectors.
* Notifiers for changes – only Telegram is supported for now.
* Configurable interval for each website.
* Simple configuration using a JSON config file.
* Experimental WebUI.

## Usage
The recommended way to run ChangeMonitor is using Docker. The below command should work well:

````
docker run -d --name changemonitor -v /path/to/config.json:/config/config.json -v /path/to/data:/data ordspilleren/changemonitor
````

Since ChangeMonitor compiles to a single binary, it will be equally easy to run it without Docker. You will find the compiled binaries in releases.

The application can be set up using the following environment variables:
* `CONFIG_FILE` Location of config file.
* `STORAGE_DIRECTORY` Location of storage directory.
* `CHROME_PATH` Location of Chrome/Chromium binary.
* `CHROME_WS` WebSocket path of Chrome DevTools.
* `ENABLE_WEBUI` Option to enable experimental WebUI.

If `CHROME_WS` is set, ChangeMonitor will try connecting to the specified URI. Otherwise, it will look for the binary specified in `CHROME_PATH`.

### Docker Compose
Below is an example of a Docker Compose setup with an external Chrome browser as a container.

````yaml
version: "3"
services:
  changemonitor:
    image: ordspilleren/changemonitor:latest
    container_name: changemonitor
    volumes:
      - type: bind
        source: /path/to/config.json
        target: /config/config.json
      - /path/to/data:/data
    environment:
      - CHROME_WS=ws://chrome:9222
    restart: unless-stopped

  chrome:
    image: zenika/alpine-chrome:latest
    container_name: chrome
    command: ["--no-sandbox", "--remote-debugging-address=0.0.0.0", "--remote-debugging-port=9222"]
    restart: unless-stopped
````
