  select genre.name, count(*) as count
    from           genre
         left join track using(genreid)
group by genre.name
order by count desc;
