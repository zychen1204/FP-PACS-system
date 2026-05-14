FROM nginx:stable-alpine

# Remove default nginx config
RUN rm /etc/nginx/conf.d/default.conf

# Copy custom nginx config
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copy static assets
COPY index.html      /usr/share/nginx/html/
COPY app.js          /usr/share/nginx/html/
COPY style.css       /usr/share/nginx/html/
COPY test-runner.html /usr/share/nginx/html/
COPY tests.js        /usr/share/nginx/html/

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
