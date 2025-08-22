bind_socket = "*:11334";
password = "mailez";
secure_ip = "{{ SUBNET }}";
{%- if SUBNET6 %}
secure_ip = "{{ SUBNET6 }}";
{%- endif %}
