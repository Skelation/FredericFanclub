# Use the lightning-fast Nginx web server
FROM nginx:alpine

# Take a snapshot of the current frontend folder and seal it inside the container
COPY ./frontend /usr/share/nginx/html

# Take a snapshot of our custom Nginx settings (the clean URLs fix)
COPY default.conf /etc/nginx/conf.d/default.conf
