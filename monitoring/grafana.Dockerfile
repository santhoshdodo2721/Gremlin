FROM grafana/grafana:10.3.3
USER root
COPY assets/gremlin-theme.css /usr/share/grafana/public/css/gremlin-theme.css
# Keep Grafana's own template and add one scoped stylesheet.
RUN sed -i 's@</head>@<link rel="stylesheet" href="public/css/gremlin-theme.css?v=4" />\n</head>@' /usr/share/grafana/public/views/index.html
USER grafana
