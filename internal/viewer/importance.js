(function(){
function importanceBadge(value,extra){var element=document.createElement('span');element.className='importance importance-'+value+(extra?' '+extra:'');element.textContent=value;return element}
function reviewBadge(value,extra){var element=document.createElement('span');element.className='review-level review-level-'+value+(extra?' '+extra:'');element.textContent=value;return element}
function fragmentID(note){var id=note.querySelector('.fragment-note-id');return id?id.textContent.split(' · ')[0].trim():''}
function markBlock(note,value){note.classList.add('fragment-block','fragment-'+value,'main-fragment-'+value);var node=note.nextElementSibling;while(node&&!node.classList.contains('fragment-note')){node.classList.add('fragment-block','fragment-'+value,'main-fragment-'+value);node=node.nextElementSibling}}
fetch('/importance.json').then(function(response){return response.json()}).then(function(data){
 document.querySelectorAll('.main-group[data-group-id]').forEach(function(group){var value=data.groups[group.dataset.groupId];if(!value)return;group.classList.add('group-'+value);var heading=group.querySelector(':scope > summary h2');if(heading)heading.after(importanceBadge(value,'group-importance'))});
 document.querySelectorAll('.nav-group > summary[data-group-id]').forEach(function(summary){var value=data.groups[summary.dataset.groupId];var title=summary.querySelector('.nav-group-title');if(value&&title)title.after(importanceBadge(value))});
 document.querySelectorAll('.file-fragment-description').forEach(function(description){var id=description.querySelector('.file-fragment-id');var value=id&&data.fragments[id.textContent.trim()];if(value){description.dataset.reviewLevel=value;description.prepend(reviewBadge(value,'fragment-review-level'))}});
 document.querySelectorAll('.fragment-note').forEach(function(note){var value=data.fragments[fragmentID(note)];if(value){note.dataset.reviewLevel=value;note.prepend(reviewBadge(value,'fragment-review-level'));markBlock(note,value)}});
}).catch(function(error){console.error('Failed to load importance data',error)});
})();
